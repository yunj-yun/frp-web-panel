package server

import (
	"fmt"

	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
)

type ValidateableServerRequest interface {
	GetServerSecret() string
	GetServerId() string
}

func ValidateServerRequest(ctx *app.Context, req ValidateableServerRequest) (*models.ServerEntity, error) {
	if req == nil {
		return nil, fmt.Errorf("invalid request")
	}

	if req.GetServerId() == "" || req.GetServerSecret() == "" {
		return nil, fmt.Errorf("invalid request")
	}

	var (
		cli *models.ServerEntity
		err error
	)

	if cli, err = dao.NewQuery(ctx).ValidateServerSecret(req.GetServerId(), req.GetServerSecret()); err != nil {
		return nil, err
	}

	return cli, nil
}
