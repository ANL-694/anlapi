package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateSourceCacheStub struct {
	data string
}

func (s *updateSourceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateSourceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateSourceClientStub struct {
	release       *GitHubRelease
	requestedRepo string
}

func (s *updateSourceClientStub) FetchLatestRelease(_ context.Context, repo string) (*GitHubRelease, error) {
	s.requestedRepo = repo
	return s.release, nil
}

func (*updateSourceClientStub) DownloadFile(context.Context, string, string, int64) error {
	return nil
}

func (*updateSourceClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	return nil, nil
}

func validANLRelease() *GitHubRelease {
	return &GitHubRelease{
		TagName: "v1.0.4",
		HTMLURL: "https://github.com/ANL-694/anl-api/releases/tag/v1.0.4",
		Assets: []GitHubAsset{{
			Name:               "anl-api_1.0.4_linux_amd64.tar.gz",
			BrowserDownloadURL: "https://github.com/ANL-694/anl-api/releases/download/v1.0.4/anl-api_1.0.4_linux_amd64.tar.gz",
		}},
	}
}

func TestFetchLatestReleaseRejectsDifferentRepository(t *testing.T) {
	client := &updateSourceClientStub{release: &GitHubRelease{
		TagName: "v0.1.146",
		HTMLURL: "https://anl-api/releases/tag/v0.1.146",
		Assets: []GitHubAsset{{
			Name:               "sub2api_0.1.146_linux_amd64.tar.gz",
			BrowserDownloadURL: "https://anl-api/releases/download/v0.1.146/sub2api_0.1.146_linux_amd64.tar.gz",
		}},
	}}
	svc := NewUpdateService(&updateSourceCacheStub{}, client, "1.0.3", "release")

	_, err := svc.fetchLatestRelease(context.Background())
	require.ErrorContains(t, err, "release source mismatch")
	require.Equal(t, githubRepo, client.requestedRepo)
}

func TestCheckUpdateIgnoresCachedReleaseFromDifferentRepository(t *testing.T) {
	maliciousCache, err := json.Marshal(struct {
		Latest      string       `json:"latest"`
		ReleaseInfo *ReleaseInfo `json:"release_info"`
		Timestamp   int64        `json:"timestamp"`
	}{
		Latest: "0.1.146",
		ReleaseInfo: &ReleaseInfo{
			HTMLURL: "https://anl-api/releases/tag/v0.1.146",
			Assets: []Asset{{
				Name:        "sub2api_0.1.146_linux_amd64.tar.gz",
				DownloadURL: "https://anl-api/releases/download/v0.1.146/sub2api_0.1.146_linux_amd64.tar.gz",
			}},
		},
		Timestamp: time.Now().Unix(),
	})
	require.NoError(t, err)

	cache := &updateSourceCacheStub{data: string(maliciousCache)}
	client := &updateSourceClientStub{release: validANLRelease()}
	svc := NewUpdateService(cache, client, "1.0.3", "release")

	info, err := svc.CheckUpdate(context.Background(), false)
	require.NoError(t, err)
	require.Equal(t, "1.0.4", info.LatestVersion)
	require.Equal(t, githubRepo, client.requestedRepo)
	require.Equal(t, validANLRelease().HTMLURL, info.ReleaseInfo.HTMLURL)
}

func TestValidateUpdateRepositoryURL(t *testing.T) {
	require.NoError(t, validateUpdateRepositoryURL("https://github.com/ANL-694/anl-api/releases/download/v1.0.4/anl-api_linux_amd64.tar.gz"))
	require.Error(t, validateUpdateRepositoryURL("https://anl-api/releases/download/v0.1.146/sub2api_linux_amd64.tar.gz"))
	require.Error(t, validateUpdateRepositoryURL("https://example.com/ANL-694/anl-api/releases/download/v1.0.4/file.tar.gz"))
}

func TestIsSupportedUpdateBinaryName(t *testing.T) {
	for _, name := range []string{"anlapi", "anlapi.exe", "ANLAPI", "anl-api", "anl-api.exe"} {
		require.True(t, isSupportedUpdateBinaryName(name), name)
	}
	for _, name := range []string{"ikik-api", "sub2api", "", "anlapi.tar"} {
		require.False(t, isSupportedUpdateBinaryName(name), name)
	}
}
