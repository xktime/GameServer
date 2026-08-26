package managers

import (
	"context"
	"fmt"
	"gameserver/common/base/actor"
	"gameserver/common/msg/message"
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
	roomDefinition            *actor.Definition[*Room, string]
	projection                participant.TeamRoomProjection
	messenger                 participant.PlayerMessenger
	now                       func() time.Time
	newRoomID                 func() string
	rooms                     map[string]*Room
	closedRooms               map[string]struct{}
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
	definition, err := actor.Define(scope, actor.Room, func(context.Context, string) (*Room, error) {
		return &Room{maxLifetime: MaxRoomLifetime, messenger: messenger}, nil
	})
	if err != nil {
		return nil, err
	}
	return &RoomManager{
		roomDefinition:            definition,
		projection:                projection,
		messenger:                 messenger,
		now:                       now,
		newRoomID:                 newRoomID,
		rooms:                     make(map[string]*Room),
		closedRooms:               make(map[string]struct{}),
		matchRooms:                make(map[string]string),
		roomMatches:               make(map[string]string),
		closedMatches:             make(map[string]time.Time),
		playerRooms:               make(map[int64]string),
		teamRooms:                 make(map[int64]string),
		desiredTeamRooms:          make(map[int64]teamRoomProjectionState),
		projectedTeamRoomVersions: make(map[int64]uint64),
	}, nil
}

func (m *RoomManager) AcceptMatch(admission matchentry.Admission) matchentry.Acceptance {
	if !validAdmission(admission) {
		return matchentry.Acceptance{
			Status:          matchentry.Rejected,
			RejectedTeamIDs: realTeamIDs(admission.Teams),
		}
	}
	m.mu.Lock()
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
	room, err := createRoom(context.Background(), m.roomDefinition, roomID, memberIDs, realPlayerIDs, teamIDs, realTeamIDs, now, m.messenger)
	if err != nil {
		m.mu.Unlock()
		log.Error("创建房间 %s 失败: %v", roomID, err)
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

	m.applyProjection(desired)
	room.send(&message.S2C_MatchResult{RoomId: roomID, PlayerInfos: playerInfos})
	return matchentry.Acceptance{Status: matchentry.Accepted, RoomID: roomID}
}

func (m *RoomManager) CloseRoom(roomID string) bool {
	m.mu.Lock()
	if _, closed := m.closedRooms[roomID]; closed {
		m.mu.Unlock()
		return true
	}
	room, exists := m.rooms[roomID]
	if !exists {
		m.mu.Unlock()
		return false
	}
	delete(m.rooms, roomID)
	m.closedRooms[roomID] = struct{}{}
	if matchID, ok := m.roomMatches[roomID]; ok {
		delete(m.roomMatches, roomID)
		delete(m.matchRooms, matchID)
		m.closedMatches[matchID] = m.now()
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
	desired := make([]teamRoomProjectionAttempt, 0, len(room.realTeamIDs))
	for _, teamID := range room.realTeamIDs {
		desired = append(desired, m.setDesiredTeamRoomLocked(teamID, ""))
	}
	m.mu.Unlock()

	room.Stop()
	m.applyProjection(desired)
	return true
}

func (m *RoomManager) HandleRecordOperate(playerID int64, roomID string, operateInfo string) *message.S2C_RecordGameOperate {
	m.mu.Lock()
	mappedRoomID, member := m.playerRooms[playerID]
	room := m.rooms[roomID]
	m.mu.Unlock()
	if !member || mappedRoomID != roomID || room == nil {
		return &message.S2C_RecordGameOperate{}
	}

	room.sendExcept(playerID, &message.S2C_RecordGameOperate{OperateInfo: operateInfo})
	return &message.S2C_RecordGameOperate{OperateInfo: operateInfo}
}

func (m *RoomManager) PlayerOffline(playerID int64) bool {
	m.mu.Lock()
	roomID, member := m.playerRooms[playerID]
	room := m.rooms[roomID]
	m.mu.Unlock()
	if !member || room == nil {
		return false
	}

	room.sendExcept(playerID, &message.S2C_PlayerOffline{PlayerId: strconv.FormatInt(playerID, 10)})
	return true
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
	m.pruneExpiredMatchTombstones()
}

func (m *RoomManager) pruneExpiredMatchTombstones() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()

	for matchID, closedAt := range m.closedMatches {
		if now.Before(closedAt.Add(MatchTombstoneTTL)) {
			continue
		}
		delete(m.closedMatches, matchID)
	}
}

func (m *RoomManager) Stop() {
	m.mu.Lock()
	roomIDs := make([]string, 0, len(m.rooms))
	for roomID := range m.rooms {
		roomIDs = append(roomIDs, roomID)
	}
	m.mu.Unlock()

	sort.Strings(roomIDs)
	for _, roomID := range roomIDs {
		m.CloseRoom(roomID)
	}
}

func (m *RoomManager) ReconcileProjections() {
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
	m.applyProjection(desired)
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

func (m *RoomManager) applyProjection(attempts []teamRoomProjectionAttempt) {
	for len(attempts) > 0 {
		desired := make([]participant.DesiredTeamRoom, len(attempts))
		for index, attempt := range attempts {
			desired[index] = attempt.desired
		}
		if err := m.projection.Apply(desired); err != nil {
			return
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
