package queryhistory

import (
	"context"
	"time"

	"github.com/grafana/grafana/pkg/models"
	"github.com/grafana/grafana/pkg/services/sqlstore"
	"github.com/grafana/grafana/pkg/util"
)

func ProvideService(sqlStore *sqlstore.SQLStore) *QueryHistoryService {
	return &QueryHistoryService{
		SQLStore: sqlStore,
	}
}

type Service interface {
	CreateQueryHistory(ctx context.Context, user *models.SignedInUser, queries string, datasourceUid string) (*models.QueryHistory, error)
	GetQueryHistory(ctx context.Context, user *models.SignedInUser, datasourceUids []string, searchString string, sort string) ([]models.QueryHistory, error)
	DeleteQueryFromQueryHistory(ctx context.Context, user *models.SignedInUser, queryId string) error
	UpdateComment(ctx context.Context, user *models.SignedInUser, query *models.QueryHistory, comment string) error
	GetQueryInQueryHistoryByUid(ctx context.Context, user *models.SignedInUser, queryId string) (*models.QueryHistory, error)
}

type QueryHistoryService struct {
	SQLStore *sqlstore.SQLStore
}

func (s QueryHistoryService) CreateQueryHistory(ctx context.Context, user *models.SignedInUser, queries string, datasourceUid string) (*models.QueryHistory, error) {
	now := time.Now().Unix()
	queryHistory := models.QueryHistory{
		OrgId:         user.OrgId,
		Uid:           util.GenerateShortUID(),
		Queries:       queries,
		DatasourceUid: datasourceUid,
		CreatedBy:     user.UserId,
		CreatedAt:     now,
		Comment:       "",
	}

	err := s.SQLStore.WithDbSession(ctx, func(session *sqlstore.DBSession) error {
		_, err := session.Insert(&queryHistory)
		return err
	})
	if err != nil {
		return nil, err
	}

	return &queryHistory, nil
}

func (s QueryHistoryService) GetQueryHistory(ctx context.Context, user *models.SignedInUser, dataSourceUids []string, searchString string, sort string) ([]models.QueryHistory, error) {
	var queryHistory []models.QueryHistory
	err := s.SQLStore.WithDbSession(ctx, func(session *sqlstore.DBSession) error {
		session.Table("query_history")
		session.In("datasource_uid", dataSourceUids)
		session.Where("org_id = ? AND created_by = ? AND queries LIKE ?", user.OrgId, user.UserId, "%"+searchString+"%")
		if sort == "time-desc" {
			session.Desc("created_at")
		} else if sort == "time-asc" {
			session.Asc("created_at")
		}
		err := session.Find(&queryHistory)
		return err
	})

	if err != nil {
		return nil, err
	}

	return queryHistory, nil
}

func (s QueryHistoryService) GetQueryInQueryHistoryByUid(ctx context.Context, user *models.SignedInUser, queryId string) (*models.QueryHistory, error) {
	var queryHistory models.QueryHistory

	err := s.SQLStore.WithDbSession(ctx, func(session *sqlstore.DBSession) error {
		exists, err := session.Where("org_id = ? AND created_by = ? AND uid = ?", user.OrgId, user.UserId, queryId).Get(&queryHistory)

		if !exists {
			return models.ErrQueryNotFound
		}

		return err
	})

	if err != nil {
		return nil, err
	}

	return &queryHistory, nil
}

func (s QueryHistoryService) DeleteQueryFromQueryHistory(ctx context.Context, user *models.SignedInUser, queryId string) error {
	err := s.SQLStore.WithDbSession(ctx, func(session *sqlstore.DBSession) error {
		id, err := session.Where("org_id = ? AND created_by = ? AND uid = ?", user.OrgId, user.UserId, queryId).Delete(models.QueryHistory{})
		if id == 0 {
			return models.ErrQueryNotFound
		}
		return err
	})

	if err != nil {
		return err
	}

	return nil
}

func (s QueryHistoryService) UpdateComment(ctx context.Context, user *models.SignedInUser, query *models.QueryHistory, comment string) error {
	query.Comment = comment
	err := s.SQLStore.WithDbSession(ctx, func(session *sqlstore.DBSession) error {
		_, err := session.ID(query.Id).Update(query)
		return err
	})

	if err != nil {
		return err
	}

	return nil
}

var _ Service = &QueryHistoryService{}
