package runner

import (
	"os"
	"path"
	"testing"

	"github.com/arkmq-org/markdown-runner/config"
	"github.com/stretchr/testify/assert"
)

func TestCheckBreakpoints(t *testing.T) {
	tmpDir := t.TempDir()
	mdContent := "```bash {\"stage\":\"stage1\"}\necho one\n```\n```bash {\"stage\":\"stage2\",\"id\":\"c\"}\necho two\n```\n"
	mdFile := path.Join(tmpDir, "test.md")
	assert.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0o644))
	failFile := path.Join(tmpDir, "fail.md")
	assert.NoError(t, os.WriteFile(failFile, []byte("```bash {\"stage\":\"deploy\"}\necho ok\n```\n```bash {\"stage\":\"boom\"}\nfalse\n```\n"), 0o644))

	check := func(files []string, start config.Location, bps ...config.Location) ([]string, error) {
		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1, View: "mock",
			StartFromLocation: start, Breakpoints: bps}
		return CheckBreakpoints(cfg, files)
	}
	one := []string{mdFile}

	t.Run("reachable breakpoints give no warning", func(t *testing.T) {
		w, err := check(one, config.Location{}, config.Location{Stage: "stage1"}, config.Location{File: "test", Stage: "stage2", Chunk: "c"})
		assert.NoError(t, err)
		assert.Empty(t, w)
	})
	t.Run("same breakpoint twice", func(t *testing.T) {
		w, err := check(one, config.Location{}, config.Location{Stage: "stage1"}, config.Location{Stage: "stage1"})
		assert.NoError(t, err)
		assert.Empty(t, w)
	})
	t.Run("unknown stage", func(t *testing.T) {
		w, err := check(one, config.Location{}, config.Location{Stage: "stge1"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"breakpoint 'stge1' is never reached: unknown stage 'stge1'"}, w)
	})
	t.Run("unknown stage when the run would fail", func(t *testing.T) {
		w, err := check([]string{failFile}, config.Location{}, config.Location{Stage: "deplyo"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"breakpoint 'deplyo' is never reached: unknown stage 'deplyo'"}, w)
	})
	t.Run("unknown chunk is an error before running", func(t *testing.T) {
		_, err := check(one, config.Location{}, config.Location{Stage: "stage2", Chunk: "nope"})
		assert.ErrorContains(t, err, "chunk with ID 'nope' not found in stage 'stage2'")
	})
	t.Run("stage before the start stage", func(t *testing.T) {
		w, err := check(one, config.Location{Stage: "stage2"}, config.Location{Stage: "stage1"})
		assert.NoError(t, err)
		assert.Len(t, w, 1)
		assert.Contains(t, w[0], "'stage1' is never reached")
		assert.Contains(t, w[0], "before the --start-from stage")
	})
	t.Run("no such file", func(t *testing.T) {
		w, err := check(one, config.Location{}, config.Location{File: "nope", Stage: "stage1"})
		assert.NoError(t, err)
		assert.Equal(t, []string{"breakpoint 'nope@stage1' is never reached: no markdown file matching 'nope' is run"}, w)
	})
	t.Run("start-from is not consumed", func(t *testing.T) {
		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1, View: "mock",
			StartFromLocation: config.Location{Stage: "stage2"}, Breakpoints: []config.Location{{Stage: "stage2"}}}
		_, err := CheckBreakpoints(cfg, one)
		assert.NoError(t, err)
		assert.Equal(t, "stage2", cfg.StartFromLocation.Stage)
	})
}
