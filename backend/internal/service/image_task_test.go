package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type imageTaskMemoryStore struct {
	task    *ImageTaskRecord
	ttl     time.Duration
	saveErr error
	getErr  error
}

type imageTaskSnapshotStorage struct {
	mu      sync.Mutex
	baseURL string
	err     error
	saves   int
}

func (s *imageTaskSnapshotStorage) Save(_ context.Context, key, _ string, _ []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.err != nil {
		return "", s.err
	}
	return s.baseURL + key, nil
}

func (s *imageTaskSnapshotStorage) saveCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saves
}

type imageTaskResolverState struct {
	mu       sync.RWMutex
	uploader *ImageResultUploader
	enabled  bool
}

func (s *imageTaskResolverState) resolve() (*ImageResultUploader, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uploader, s.enabled
}

func (s *imageTaskResolverState) set(uploader *ImageResultUploader, enabled bool) {
	s.mu.Lock()
	s.uploader = uploader
	s.enabled = enabled
	s.mu.Unlock()
}

func imageTaskBase64Result() json.RawMessage {
	b64 := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nimage-task-snapshot"))
	return json.RawMessage(`{"data":[{"b64_json":"` + b64 + `"}]}`)
}

func (s *imageTaskMemoryStore) Save(_ context.Context, task *ImageTaskRecord, ttl time.Duration) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	copy := *task
	s.task = &copy
	s.ttl = ttl
	return nil
}

func (s *imageTaskMemoryStore) Get(_ context.Context, _ string) (*ImageTaskRecord, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.task == nil {
		return nil, ErrImageTaskNotFound
	}
	copy := *s.task
	return &copy, nil
}

func TestImageTaskServiceLifecycleAndOwnership(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, 10*time.Minute)
	owner := ImageTaskOwner{UserID: 7, APIKeyID: 9}

	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusProcessing, created.Status)
	require.Equal(t, created.ID, created.TaskID)
	require.Equal(t, "image.generation.task", created.Object)
	require.Equal(t, time.Hour, store.ttl)
	require.Equal(t, owner.UserID, store.task.UserID)
	require.Equal(t, owner.APIKeyID, store.task.APIKeyID)

	_, err = svc.Get(context.Background(), ImageTaskOwner{UserID: 7, APIKeyID: 10}, created.ID)
	require.ErrorIs(t, err, ErrImageTaskNotFound)

	result := json.RawMessage(`{"created":123,"data":[{"url":"https://example.test/image.png"}]}`)
	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, result))

	completed, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusCompleted, completed.Status)
	require.Equal(t, http.StatusOK, completed.HTTPStatus)
	require.Equal(t, "https://example.test/image.png", completed.ImageURL)
	require.JSONEq(t, string(result), string(completed.Result))
	require.NotNil(t, completed.CompletedAt)
}

func TestImageTaskServiceInvalidResultBecomesFailed(t *testing.T) {
	store := &imageTaskMemoryStore{}
	svc := NewImageTaskServiceWithOptions(store, time.Hour, time.Minute)
	created, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2})
	require.NoError(t, err)

	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, json.RawMessage(`not-json`)))
	got, err := svc.Get(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2}, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusFailed, got.Status)
	require.Equal(t, http.StatusBadGateway, got.HTTPStatus)
	require.Contains(t, string(got.Error), "non-JSON")
}

func TestImageTaskServiceMapsStoreFailures(t *testing.T) {
	store := &imageTaskMemoryStore{saveErr: errors.New("redis down")}
	svc := NewImageTaskService(store)

	_, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 1, APIKeyID: 2})
	require.ErrorIs(t, err, ErrImageTaskUnavailable)
}

func TestImageTaskServiceUsesUploaderCapturedAtCreate(t *testing.T) {
	for _, tc := range []struct {
		name        string
		replacement bool
	}{
		{name: "storage disabled"},
		{name: "storage replaced", replacement: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &imageTaskMemoryStore{}
			capturedStorage := &imageTaskSnapshotStorage{baseURL: "https://captured.example/"}
			capturedUploader := NewImageResultUploader(capturedStorage, "captured/", 0, nil)
			state := &imageTaskResolverState{uploader: capturedUploader, enabled: true}
			svc := NewImageTaskServiceWithResolver(store, state.resolve, time.Hour, time.Minute)

			owner := ImageTaskOwner{UserID: 11, APIKeyID: 12}
			created, err := svc.Create(context.Background(), owner)
			require.NoError(t, err)

			var replacementStorage *imageTaskSnapshotStorage
			if tc.replacement {
				replacementStorage = &imageTaskSnapshotStorage{baseURL: "https://replacement.example/"}
				state.set(NewImageResultUploader(replacementStorage, "replacement/", 0, nil), true)
			} else {
				state.set(nil, false)
			}

			require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, imageTaskBase64Result()))
			got, err := svc.Get(context.Background(), owner, created.ID)
			require.NoError(t, err)
			require.Equal(t, ImageTaskStatusCompleted, got.Status)
			require.Contains(t, got.ImageURL, "https://captured.example/captured/")
			require.NotContains(t, string(got.Result), "b64_json")
			require.Equal(t, 1, capturedStorage.saveCount())
			_, retained := svc.taskUploaders.Load(created.ID)
			require.False(t, retained)
			if replacementStorage != nil {
				require.Zero(t, replacementStorage.saveCount())
			}
		})
	}
}

func TestImageTaskServiceCapturedUploaderFailureMarksTaskFailed(t *testing.T) {
	store := &imageTaskMemoryStore{}
	capturedStorage := &imageTaskSnapshotStorage{
		baseURL: "https://captured.example/",
		err:     errors.New("captured bucket unavailable"),
	}
	state := &imageTaskResolverState{
		uploader: NewImageResultUploader(capturedStorage, "captured/", 0, nil),
		enabled:  true,
	}
	svc := NewImageTaskServiceWithResolver(store, state.resolve, time.Hour, time.Minute)
	owner := ImageTaskOwner{UserID: 21, APIKeyID: 22}
	created, err := svc.Create(context.Background(), owner)
	require.NoError(t, err)

	replacementStorage := &imageTaskSnapshotStorage{baseURL: "https://replacement.example/"}
	state.set(NewImageResultUploader(replacementStorage, "replacement/", 0, nil), true)

	require.NoError(t, svc.Complete(context.Background(), created.ID, http.StatusOK, imageTaskBase64Result()))
	got, err := svc.Get(context.Background(), owner, created.ID)
	require.NoError(t, err)
	require.Equal(t, ImageTaskStatusFailed, got.Status)
	require.Equal(t, http.StatusBadGateway, got.HTTPStatus)
	require.Contains(t, string(got.Error), "object storage")
	require.NotContains(t, string(got.Result), "b64_json")
	require.Equal(t, 1, capturedStorage.saveCount())
	require.Zero(t, replacementStorage.saveCount())
	_, retained := svc.taskUploaders.Load(created.ID)
	require.False(t, retained)
}

func TestImageTaskServiceCreateRejectsDisabledResolverAfterEnablementCheck(t *testing.T) {
	store := &imageTaskMemoryStore{}
	storage := &imageTaskSnapshotStorage{baseURL: "https://captured.example/"}
	state := &imageTaskResolverState{
		uploader: NewImageResultUploader(storage, "captured/", 0, nil),
		enabled:  true,
	}
	svc := NewImageTaskServiceWithResolver(store, state.resolve, time.Hour, time.Minute)
	require.True(t, svc.Enabled())

	state.set(nil, false)
	_, err := svc.Create(context.Background(), ImageTaskOwner{UserID: 31, APIKeyID: 32})
	require.ErrorIs(t, err, ErrImageTaskUnavailable)
	require.Nil(t, store.task)
}
