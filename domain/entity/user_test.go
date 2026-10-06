package entity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/not-for-prod/proterror/proterror"

	"github.com/not-for-prod/authorization/domain/entity"
)

func TestUserDelete(t *testing.T) {
	for _, status := range []entity.UserStatus{entity.UserStatusActive, entity.UserStatusDeleted, "disabled", ""} {
		t.Run(string(status), func(t *testing.T) {
			user := entity.User{ID: "user", Status: status, CreatedAt: time.Now().UTC()}
			before := user
			err := user.Delete()
			if status == entity.UserStatusActive {
				if err != nil || user.Status != entity.UserStatusDeleted || user.ID != before.ID ||
					user.CreatedAt != before.CreatedAt {
					t.Fatalf("delete: %#v %v", user, err)
				}
			} else if !errors.Is(err, &proterror.FailedPrecondition{}) || user != before {
				t.Fatalf("invalid transition: %#v %v", user, err)
			}
		})
	}
}
