package service

import (
	activitydb "github.com/findardi/rakda/server/internal/activity/repository/sqlc"
)

type ActivityRepository = activitydb.Querier
