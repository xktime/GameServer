package team

import "gameserver/common/db/mongodb"

type teamRepository interface {
	FindByID(teamID int64) (*Team, error)
	Save(team *Team) error
	DeleteByID(teamID int64) error
}

type mongoTeamRepository struct{}

func (mongoTeamRepository) FindByID(teamID int64) (*Team, error) {
	return mongodb.FindOneById[Team](teamID)
}

func (mongoTeamRepository) Save(team *Team) error {
	_, err := mongodb.Save(team)
	return err
}

func (mongoTeamRepository) DeleteByID(teamID int64) error {
	_, err := mongodb.DeleteByID[Team](teamID)
	return err
}
