package team

import (
	"context"
	"gameserver/common/db/mongodb"
)

type Repository interface {
	FindByID(ctx context.Context, teamID int64) (*Team, error)
	Save(ctx context.Context, team *Team) error
	DeleteByID(ctx context.Context, teamID int64) error
}

type mongoTeamRepository struct{}

func (mongoTeamRepository) FindByID(ctx context.Context, teamID int64) (*Team, error) {
	return mongodb.FindOneByIDContext[Team](ctx, teamID)
}

func (mongoTeamRepository) Save(ctx context.Context, team *Team) error {
	_, err := mongodb.SaveContext(ctx, team)
	return err
}

func (mongoTeamRepository) DeleteByID(ctx context.Context, teamID int64) error {
	_, err := mongodb.DeleteByIDContext[Team](ctx, teamID)
	return err
}
