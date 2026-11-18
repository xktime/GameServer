package managers

import (
	"context"
	"errors"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
	"gameserver/common/utils"
	"gameserver/core/log"
	"gameserver/modules/room/matchentry"
	"gameserver/modules/room/participant"
	"sort"
	"strconv"
	"sync"
	"time"
)

type RoomManager struct {
	mu                        sync.Mutex
	recoveryMu                utils.KeyedMutex[string]
	stopped                   bool
	roomDefinition            *actor.Definition[*roomActor, string]
	projection                participant.TeamRoomProjection
	now                       func() time.Time
	newRoomID                 func() string
	rooms                     map[string]*roomActor
	closedRooms               map[string]closedRoomState
	stoppingRooms             map[string]actor.ActorRef
	matchRooms                map[string]string
	roomMatches               map[string]string
	closedMatches             map[string]time.Time
	playerRooms               map[int64]string
	teamRooms                 map[int64]string
	desiredTeamRooms          map[int64]teamRoomProjectionState
	projectedTeamRoomVersions map[int64]uint64
	nextProjectionVersion     uint64
}

const MatchTombstoneTTL = 5 * time.Minute

type closedRoomState struct {
	teamIDs  []int64
	closedAt time.Time
}

type teamRoomProjectionState struct {
	roomID  string
	version uint64
}

type teamRoomProjectionAttempt struct {
	desired participant.DesiredTeamRoom
	version uint64
}

func NewRoomManager(
	scope *actor.Scope,
	projection participant.TeamRoomProjection,
	messenger participant.PlayerMessenger,
	now func() time.Time,
	newRoomID func() string,
) (*RoomManager, error) {
	if projection == nil {
		return nil, fmt.Errorf("room: TeamRoomProjection is nil")
	}
	if messenger == nil {
		return nil, fmt.Errorf("room: PlayerMessenger is nil")
	}
	if now == nil {
		return nil, fmt.Errorf("room: Clock is nil")
	}
	if newRoomID == nil {
		return nil, fmt.Errorf("room: RoomID generator is nil")
	}
	definition, err := actor.Define(scope, func(context.Context, string) (*roomActor, error) {
		return &roomActor{maxLifetime: MaxRoomLifetime, messenger: messenger}, nil
	}, actor.DefinitionOptions{QueueCapacity: 256})
	if err != nil {
		return nil, err
	}
	return &RoomManager{
		roomDefinition:            definition,
		projection:                projection,
		now:                       now,
		newRoomID:                 newRoomID,
		rooms:                     make(map[string]*roomActor),
		closedRooms:               make(map[string]closedRoomState),
		stoppingRooms:             make(map[string]actor.ActorRef),
		matchRooms:                make(map[string]string),
		roomMatches:               make(map[string]string),
		closedMatches:             make(map[string]time.Time),
		playerRooms:               make(map[int64]string),
		teamRooms:                 make(map[int64]string),
		desiredTeamRooms:          make(map[int64]teamRoomProjectionState),
		projectedTeamRoomVersions: make(map[int64]uint64),
	}, nil
}

func (m *RoomManager) AcceptMatch(ctx context.Context, admission matchentry.Admission) matchentry.Acceptance {
	if !validAdmission(admission) {
		return matchentry.Acceptance{
			Status:          matchentry.Rejected,
			RejectedTeamIDs: realTeamIDs(admission.Teams),
		}
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return matchentry.Acceptance{Status: matchentry.Retryable}
	}
	if roomID, ok := m.matchRooms[admission.MatchID]; ok {
		m.mu.Unlock()
		return matchentry.Acceptance{Status: matchentry.AlreadyAccepted, RoomID: roomID}
	}
	now := m.now()
	if closedAt, ok := m.closedMatches[admission.MatchID]; ok {
		if now.Before(closedAt.Add(MatchTombstoneTTL)) {
			m.mu.Unlock()
			return matchentry.Acceptance{
				Status:          matchentry.Rejected,
				RejectedTeamIDs: realTeamIDs(admission.Teams),
			}
		}
		delete(m.closedMatches, admission.MatchID)
	}
	conflictingTeamIDs := make([]int64, 0)
	for _, team := range admission.Teams {
		_, teamConflict := m.teamRooms[team.TeamID]
		playerConflict := false
		for _, playerID := range team.PlayerIDs {
			if _, exists := m.playerRooms[playerID]; exists {
				playerConflict = true
				break
			}
		}
		if teamConflict || playerConflict {
			conflictingTeamIDs = append(conflictingTeamIDs, team.TeamID)
		}
	}
	if len(conflictingTeamIDs) > 0 {
		m.mu.Unlock()
		return matchentry.Acceptance{
			Status:          matchentry.Rejected,
			RejectedTeamIDs: conflictingTeamIDs,
		}
	}
	roomID := m.newRoomID()
	memberIDs := make([]int64, 0)
	realPlayerIDs := make([]int64, 0)
	teamIDs := make([]int64, 0, len(admission.Teams))
	realTeamIDs := make([]int64, 0, len(admission.Teams))
	playerInfos := make([]*message.MatchPlayerInfo, 0)
	for _, team := range admission.Teams {
		teamIDs = append(teamIDs, team.TeamID)
		memberIDs = append(memberIDs, team.PlayerIDs...)
		if !team.IsRobot {
			realTeamIDs = append(realTeamIDs, team.TeamID)
			realPlayerIDs = append(realPlayerIDs, team.PlayerIDs...)
		}
		for _, playerID := range team.PlayerIDs {
			playerInfos = append(playerInfos, &message.MatchPlayerInfo{
				PlayerId: strconv.FormatInt(playerID, 10),
				IsRobot:  team.IsRobot,
			})
		}
	}
	room, err := createRoom(ctx, m.roomDefinition, roomID, memberIDs, realPlayerIDs, teamIDs, realTeamIDs, now)
	if err != nil {
		m.mu.Unlock()
		// 初始化由 Scope 管理，调用方取消等待后仍可能生成实例。
		cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
		cleanupErr := m.roomDefinition.Stop(cleanupContext, roomID)
		cancelCleanup()
		log.Error("创建房间 %s 失败: %v", roomID, errors.Join(err, cleanupErr))
		return matchentry.Acceptance{Status: matchentry.Retryable}
	}
	m.rooms[roomID] = room
	m.matchRooms[admission.MatchID] = roomID
	m.roomMatches[roomID] = admission.MatchID
	for _, playerID := range memberIDs {
		m.playerRooms[playerID] = roomID
	}
	for _, teamID := range teamIDs {
		m.teamRooms[teamID] = roomID
	}
	desired := make([]teamRoomProjectionAttempt, 0, len(realTeamIDs))
	for _, teamID := range realTeamIDs {
		desired = append(desired, m.setDesiredTeamRoomLocked(teamID, roomID))
	}
	m.mu.Unlock()

	// 房间已提交，后续投影和通知不再受调用方取消影响，同时保留 Actor 调用链。
	committedContext := context.WithoutCancel(ctx)
	m.applyProjection(committedContext, desired)
	room.send(committedContext, &message.S2C_MatchResult{RoomId: roomID, PlayerInfos: playerInfos})
	return matchentry.Acceptance{Status: matchentry.Accepted, RoomID: roomID}
}

// CloseRoom 的完成记录保留一个墓碑周期，未完成的清理继续保留以供重试。
func (m *RoomManager) CloseRoom(roomID string) bool {
	closed, err := m.closeRoom(context.Background(), roomID, false)
	if err != nil {
		log.Error("关闭房间 %s 失败: %v", roomID, err)
	}
	return closed
}

func (m *RoomManager) closeRoom(ctx context.Context, roomID string, quarantined bool) (bool, error) {
	m.mu.Lock()
	var desired []teamRoomProjectionAttempt
	if closed, exists := m.closedRooms[roomID]; exists {
		desired = make([]teamRoomProjectionAttempt, 0, len(closed.teamIDs))
		for _, teamID := range closed.teamIDs {
			state := m.desiredTeamRooms[teamID]
			if m.projectedTeamRoomVersions[teamID] < state.version {
				desired = append(desired, teamRoomProjectionAttempt{
					desired: participant.DesiredTeamRoom{TeamID: teamID, RoomID: state.roomID},
					version: state.version,
				})
			}
		}
	} else {
		room, exists := m.rooms[roomID]
		if !exists {
			m.mu.Unlock()
			return false, nil
		}
		delete(m.rooms, roomID)
		// 保留清理涉及的 Team，房间索引删除后仍可单独重试其投影。
		closedAt := m.now()
		m.closedRooms[roomID] = closedRoomState{teamIDs: room.realTeamIDs, closedAt: closedAt}
		m.stoppingRooms[roomID] = room.Ref()
		if matchID, ok := m.roomMatches[roomID]; ok {
			delete(m.roomMatches, roomID)
			delete(m.matchRooms, matchID)
			m.closedMatches[matchID] = closedAt
		}
		for _, playerID := range room.memberIDs {
			if m.playerRooms[playerID] == roomID {
				delete(m.playerRooms, playerID)
			}
		}
		for _, teamID := range room.teamIDs {
			if m.teamRooms[teamID] == roomID {
				delete(m.teamRooms, teamID)
			}
		}
		desired = make([]teamRoomProjectionAttempt, 0, len(room.realTeamIDs))
		for _, teamID := range room.realTeamIDs {
			desired = append(desired, m.setDesiredTeamRoomLocked(teamID, ""))
		}
	}
	ref, pending := m.stoppingRooms[roomID]
	m.mu.Unlock()

	var stopErr error
	if pending && !quarantined {
		stopErr = m.stopRoomActor(ctx, roomID, ref)
	}
	projectionErr := m.applyProjection(ctx, desired)
	return true, errors.Join(stopErr, projectionErr)
}

func (m *RoomManager) stopRoomActor(ctx context.Context, roomID string, ref actor.ActorRef) error {
	if err := ref.Stop(ctx); err != nil {
		return err
	}
	m.markRoomStopped(roomID)
	return nil
}

func (m *RoomManager) markRoomStopped(roomID string) {
	m.mu.Lock()
	delete(m.stoppingRooms, roomID)
	m.mu.Unlock()
}

func (m *RoomManager) HandleRecordOperate(playerID int64, roomID string, operateInfo string) *message.S2C_RecordGameOperate {
	m.mu.Lock()
	mappedRoomID, member := m.playerRooms[playerID]
	room := m.rooms[roomID]
	m.mu.Unlock()
	if !member || mappedRoomID != roomID || room == nil {
		return &message.S2C_RecordGameOperate{}
	}

	if err := room.sendExcept(context.Background(), playerID, &message.S2C_RecordGameOperate{OperateInfo: operateInfo}); err != nil {
		log.Error("发送房间操作失败: %v", err)
		return &message.S2C_RecordGameOperate{}
	}
	return &message.S2C_RecordGameOperate{OperateInfo: operateInfo}
}

func (m *RoomManager) PlayerOffline(ctx context.Context, playerID int64) bool {
	m.mu.Lock()
	roomID, member := m.playerRooms[playerID]
	room := m.rooms[roomID]
	m.mu.Unlock()
	if !member || room == nil {
		return false
	}

	// UserManager calls this only after closing the current Session. Wait until
	// dispatch has actually run, so a timed-out Room task cannot run after the
	// next login has already committed. Delivery remains best effort.
	err := room.sendExcept(ctx, playerID, &message.S2C_PlayerOffline{PlayerId: strconv.FormatInt(playerID, 10)})
	if err != nil {
		log.Error("派发玩家离线失败: player=%d error=%v", playerID, err)
	}
	return err == nil
}

func (m *RoomManager) CloseExpired() int {
	now := m.now()
	m.mu.Lock()
	roomIDs := make([]string, 0)
	for roomID, room := range m.rooms {
		if !now.Before(room.createdAt.Add(room.maxLifetime)) {
			roomIDs = append(roomIDs, roomID)
		}
	}
	m.mu.Unlock()

	sort.Strings(roomIDs)
	for _, roomID := range roomIDs {
		m.CloseRoom(roomID)
	}
	return len(roomIDs)
}

func (m *RoomManager) Maintain() {
	m.CloseExpired()
	m.ReconcileProjections()
	m.pruneExpiredTombstones()
}

func (m *RoomManager) pruneExpiredTombstones() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for matchID, closedAt := range m.closedMatches {
		if now.Before(closedAt.Add(MatchTombstoneTTL)) {
			continue
		}
		delete(m.closedMatches, matchID)
	}
	for roomID, closed := range m.closedRooms {
		if now.Before(closed.closedAt.Add(MatchTombstoneTTL)) {
			continue
		}
		if _, stopping := m.stoppingRooms[roomID]; stopping {
			continue
		}
		projected := true
		for _, teamID := range closed.teamIDs {
			if m.projectedTeamRoomVersions[teamID] < m.desiredTeamRooms[teamID].version {
				projected = false
				break
			}
		}
		if projected {
			delete(m.closedRooms, roomID)
		}
	}
}

func (m *RoomManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	m.stopped = true
	roomIDs := make([]string, 0, len(m.rooms)+len(m.stoppingRooms))
	for roomID := range m.rooms {
		roomIDs = append(roomIDs, roomID)
	}
	for roomID := range m.stoppingRooms {
		roomIDs = append(roomIDs, roomID)
	}
	m.mu.Unlock()

	sort.Strings(roomIDs)
	var failures []error
	for _, roomID := range roomIDs {
		if _, err := m.closeRoom(ctx, roomID, false); err != nil {
			failures = append(failures, err)
		}
	}
	if err := m.reconcileProjections(ctx); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (m *RoomManager) ReconcileProjections() {
	m.reconcileProjections(context.Background())
}

func (m *RoomManager) reconcileProjections(ctx context.Context) error {
	m.mu.Lock()
	desired := make([]teamRoomProjectionAttempt, 0, len(m.desiredTeamRooms))
	for teamID, state := range m.desiredTeamRooms {
		if m.projectedTeamRoomVersions[teamID] < state.version {
			desired = append(desired, teamRoomProjectionAttempt{
				desired: participant.DesiredTeamRoom{TeamID: teamID, RoomID: state.roomID},
				version: state.version,
			})
		}
	}
	m.mu.Unlock()

	sortProjectionAttempts(desired)
	return m.applyProjection(ctx, desired)
}

func (m *RoomManager) setDesiredTeamRoomLocked(teamID int64, roomID string) teamRoomProjectionAttempt {
	m.nextProjectionVersion++
	state := teamRoomProjectionState{roomID: roomID, version: m.nextProjectionVersion}
	m.desiredTeamRooms[teamID] = state
	return teamRoomProjectionAttempt{
		desired: participant.DesiredTeamRoom{TeamID: teamID, RoomID: roomID},
		version: state.version,
	}
}

func (m *RoomManager) applyProjection(ctx context.Context, attempts []teamRoomProjectionAttempt) error {
	for len(attempts) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		desired := make([]participant.DesiredTeamRoom, len(attempts))
		for index, attempt := range attempts {
			desired[index] = attempt.desired
		}
		if err := m.projection.Apply(ctx, desired); err != nil {
			return err
		}

		latestByTeam := make(map[int64]teamRoomProjectionAttempt)
		m.mu.Lock()
		for _, attempt := range attempts {
			teamID := attempt.desired.TeamID
			state, exists := m.desiredTeamRooms[teamID]
			if !exists {
				continue
			}
			if state.version == attempt.version {
				m.projectedTeamRoomVersions[teamID] = attempt.version
				continue
			}
			delete(m.projectedTeamRoomVersions, teamID)
			latestByTeam[teamID] = teamRoomProjectionAttempt{
				desired: participant.DesiredTeamRoom{TeamID: teamID, RoomID: state.roomID},
				version: state.version,
			}
		}
		m.mu.Unlock()

		attempts = attempts[:0]
		for _, attempt := range latestByTeam {
			attempts = append(attempts, attempt)
		}
		sortProjectionAttempts(attempts)
	}
	return nil
}

func sortProjectionAttempts(attempts []teamRoomProjectionAttempt) {
	sort.Slice(attempts, func(i, j int) bool {
		return attempts[i].desired.TeamID < attempts[j].desired.TeamID
	})
}

func validAdmission(admission matchentry.Admission) bool {
	if admission.MatchID == "" || len(admission.Teams) == 0 {
		return false
	}
	teamIDs := make(map[int64]struct{}, len(admission.Teams))
	playerIDs := make(map[int64]struct{})
	hasRealTeam := false
	for _, team := range admission.Teams {
		if team.TeamID == 0 || len(team.PlayerIDs) == 0 {
			return false
		}
		if _, exists := teamIDs[team.TeamID]; exists {
			return false
		}
		teamIDs[team.TeamID] = struct{}{}
		if !team.IsRobot {
			hasRealTeam = true
		}
		for _, playerID := range team.PlayerIDs {
			if playerID == 0 {
				return false
			}
			if _, exists := playerIDs[playerID]; exists {
				return false
			}
			playerIDs[playerID] = struct{}{}
		}
	}
	return hasRealTeam
}

func realTeamIDs(teams []matchentry.MatchedTeam) []int64 {
	teamIDs := make([]int64, 0, len(teams))
	seen := make(map[int64]struct{}, len(teams))
	for _, team := range teams {
		if team.IsRobot {
			continue
		}
		if _, exists := seen[team.TeamID]; exists {
			continue
		}
		seen[team.TeamID] = struct{}{}
		teamIDs = append(teamIDs, team.TeamID)
	}
	return teamIDs
}
