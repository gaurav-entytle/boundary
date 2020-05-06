package iam

import (
	"context"
	"reflect"
	"testing"

	wrapping "github.com/hashicorp/go-kms-wrapping"
	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/watchtower/internal/db"
	"github.com/hashicorp/watchtower/internal/oplog"
	"github.com/hashicorp/watchtower/internal/oplog/store"
	"github.com/stretchr/testify/assert"
)

func TestNewDatabaseRepository(t *testing.T) {
	t.Parallel()
	cleanup, conn := db.TestSetup(t, "../db/migrations/postgres")
	defer cleanup()
	assert := assert.New(t)
	defer conn.Close()

	rw := &db.GormReadWriter{Tx: conn}
	wrapper := db.TestWrapper(t)
	type args struct {
		r       db.Reader
		w       db.Writer
		wrapper wrapping.Wrapper
	}
	tests := []struct {
		name          string
		args          args
		want          Repository
		wantErr       bool
		wantErrString string
	}{
		{
			name: "valid",
			args: args{
				r:       rw,
				w:       rw,
				wrapper: wrapper,
			},
			want: &dbRepository{
				reader:  rw,
				writer:  rw,
				wrapper: wrapper,
			},
			wantErr: false,
		},
		{
			name: "nil-wrapper",
			args: args{
				r:       rw,
				w:       rw,
				wrapper: nil,
			},
			want:          nil,
			wantErr:       true,
			wantErrString: "error creating db repository with nil wrapper",
		},
		{
			name: "nil-writer",
			args: args{
				r:       rw,
				w:       nil,
				wrapper: wrapper,
			},
			want:          nil,
			wantErr:       true,
			wantErrString: "error creating db repository with nil writer",
		},
		{
			name: "nil-reader",
			args: args{
				r:       nil,
				w:       rw,
				wrapper: wrapper,
			},
			want:          nil,
			wantErr:       true,
			wantErrString: "error creating db repository with nil reader",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewRepository(tt.args.r, tt.args.w, tt.args.wrapper)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDatabaseRepository() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewDatabaseRepository() = %v, want %v", got, tt.want)
			}
			if err != nil {
				assert.Equal(err.Error(), tt.wantErrString)
			}
		})
	}
}
func Test_dbRepository_create(t *testing.T) {
	t.Parallel()
	cleanup, conn := db.TestSetup(t, "../db/migrations/postgres")
	defer cleanup()
	assert := assert.New(t)
	defer conn.Close()

	t.Run("valid-scope", func(t *testing.T) {
		rw := &db.GormReadWriter{Tx: conn}
		wrapper := db.TestWrapper(t)
		repo, err := NewRepository(rw, rw, wrapper)
		id, err := uuid.GenerateUUID()
		assert.Nil(err)

		s, err := NewScope(OrganizationScope, WithName("fname-"+id))
		retScope, err := repo.create(context.Background(), s)
		assert.Nil(err)
		assert.True(retScope != nil)
		assert.True(retScope.GetPublicId() != "")
		assert.Equal(retScope.GetName(), "fname-"+id)

		foundScope, err := repo.LookupScope(context.Background(), WitPublicId(s.PublicId))
		assert.Nil(err)
		assert.Equal(foundScope.GetPublicId(), retScope.GetPublicId())

		foundScope.Name = "fname-" + id
		foundScope, err = repo.LookupScope(context.Background(), WithName("fname-"+id))
		assert.Nil(err)
		assert.Equal(foundScope.GetPublicId(), retScope.GetPublicId())

		var metadata store.Metadata
		err = conn.Where("key = ? and value = ?", "resource-public-id", s.PublicId).First(&metadata).Error
		assert.Nil(err)

		var foundEntry oplog.Entry
		err = conn.Where("id = ?", metadata.EntryId).First(&foundEntry).Error
		assert.Nil(err)
	})
	t.Run("valid-user", func(t *testing.T) {
		rw := &db.GormReadWriter{Tx: conn}
		wrapper := db.TestWrapper(t)
		repo, err := NewRepository(rw, rw, wrapper)

		s, err := NewScope(OrganizationScope)
		retScope, err := repo.create(context.Background(), s)
		assert.Nil(err)
		assert.True(retScope != nil)
		assert.True(retScope.GetPublicId() != "")
		assert.True(retScope.GetCreateTime() != nil)
		assert.True(retScope.GetUpdateTime() != nil)

		user, err := NewUser(retScope.(*Scope))
		assert.Nil(err)
		retUser, err := repo.create(context.Background(), user)
		assert.Nil(err)
		assert.True(retUser != nil)
		assert.True(retUser.GetPublicId() != "")
		assert.Equal(retUser.(*User).PrimaryScopeId, retScope.(*Scope).PublicId)

		var metadata store.Metadata
		err = conn.Where("key = ? and value = ?", "resource-public-id", user.PublicId).First(&metadata).Error
		assert.Nil(err)

		var foundEntry oplog.Entry
		err = conn.Where("id = ?", metadata.GetEntryId()).First(&foundEntry).Error
		assert.Nil(err)
	})
	t.Run("nil-resource", func(t *testing.T) {
		rw := &db.GormReadWriter{Tx: conn}
		wrapper := db.TestWrapper(t)
		repo, err := NewRepository(rw, rw, wrapper)
		resource, err := repo.create(context.Background(), nil)
		assert.True(err != nil)
		assert.True(resource == nil)
		assert.Equal(err.Error(), "error creating resource that is nil")
	})
}

func Test_dbRepository_update(t *testing.T) {
	t.Parallel()
	cleanup, conn := db.TestSetup(t, "../db/migrations/postgres")
	defer cleanup()
	assert := assert.New(t)
	defer conn.Close()

	t.Run("valid-scope", func(t *testing.T) {
		rw := &db.GormReadWriter{Tx: conn}
		wrapper := db.TestWrapper(t)
		repo, err := NewRepository(rw, rw, wrapper)
		id, err := uuid.GenerateUUID()
		assert.Nil(err)

		s, err := NewScope(OrganizationScope)
		retScope, err := repo.create(context.Background(), s)
		assert.Nil(err)
		assert.True(retScope != nil)
		assert.True(retScope.GetPublicId() != "")
		assert.Equal(retScope.GetName(), "")

		retScope.(*Scope).Name = "fname-" + id
		retScope, err = repo.update(context.Background(), retScope, []string{"Name"})
		assert.Nil(err)
		assert.True(retScope != nil)
		assert.Equal(retScope.GetName(), "fname-"+id)

		foundScope, err := repo.LookupScope(context.Background(), WithName("fname-"+id))
		assert.Nil(err)
		assert.Equal(foundScope.GetPublicId(), retScope.GetPublicId())

		var metadata store.Metadata
		err = conn.Where("key = ? and value = ?", "resource-public-id", s.PublicId).First(&metadata).Error
		assert.Nil(err)

		var foundEntry oplog.Entry
		err = conn.Where("id = ?", metadata.EntryId).First(&foundEntry).Error
		assert.Nil(err)
	})
	t.Run("nil-resource", func(t *testing.T) {
		rw := &db.GormReadWriter{Tx: conn}
		wrapper := db.TestWrapper(t)
		repo, err := NewRepository(rw, rw, wrapper)
		resource, err := repo.update(context.Background(), nil, nil)
		assert.True(err != nil)
		assert.True(resource == nil)
		assert.Equal(err.Error(), "error updating resource that is nil")
	})
}