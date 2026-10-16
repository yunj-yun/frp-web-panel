package auth

import (
	"github.com/yunj-yun/frp-web-panel/conf"
	"github.com/yunj-yun/frp-web-panel/defs"
	"github.com/yunj-yun/frp-web-panel/middleware"
	"github.com/yunj-yun/frp-web-panel/models"
	"github.com/yunj-yun/frp-web-panel/pb"
	"github.com/yunj-yun/frp-web-panel/services/app"
	"github.com/yunj-yun/frp-web-panel/services/dao"
	"github.com/yunj-yun/frp-web-panel/utils/logger"
)

func LoginHandler(ctx *app.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	username := req.GetUsername()
	password := req.GetPassword()
	ok, user, err := dao.NewQuery(ctx).CheckUserPassword(username, password)
	if err != nil {
		return nil, err
	}

	if !ok {
		return &pb.LoginResponse{
			Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "invalid username or password"},
		}, nil
	}

	userCount, err := dao.NewQuery(ctx).AdminCountUsers()
	if err != nil {
		logger.Logger(ctx).WithError(err).Error("get user count failed")
	}

	if userCount == 1 && user.GetSafeUserInfo().Role != defs.UserRole_Admin {
		userEntity, ok := user.(models.User)
		if !ok {
			logger.Logger(ctx).Errorf("trans user entity failed, invalid user entity")
			return &pb.LoginResponse{
				Status: &pb.Status{Code: pb.RespCode_RESP_CODE_INVALID, Message: "invalid user"},
			}, nil
		}

		userEntity.Role = defs.UserRole_Admin

		dao.NewMutation(ctx).AdminUpdateUser(&models.UserEntity{
			UserID: user.GetUserID(),
		}, userEntity.UserEntity)
	}

	tokenStr := conf.GetJWTWithAllPermission(ctx.GetApp().GetConfig(), user.GetUserID())

	ginCtx := ctx.GetGinCtx()
	middleware.PushTokenStr(ginCtx, ctx.GetApp(), tokenStr)

	return &pb.LoginResponse{
		Status: &pb.Status{Code: pb.RespCode_RESP_CODE_SUCCESS, Message: "ok"},
		Token:  &tokenStr,
	}, nil
}
