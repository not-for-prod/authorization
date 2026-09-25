package convert

import (
	"github.com/not-for-prod/authorization/domain/entity"
	"github.com/not-for-prod/authorization/generated/xo"
)

func UserToXO(u entity.User) xo.AuthUser {
	return xo.AuthUser{ID: string(u.ID), Status: string(u.Status), CreatedAt: u.CreatedAt}
}
func UserFromXO(u xo.AuthUser) entity.User {
	return entity.User{ID: entity.UserID(u.ID), Status: entity.UserStatus(u.Status), CreatedAt: u.CreatedAt.UTC()}
}
