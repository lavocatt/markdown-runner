package runner

import (
	"os"
	"path"
	"testing"

	"github.com/arkmq-org/markdown-runner/config"
	"github.com/stretchr/testify/assert"
)

func TestUnreachedBreakpoints(t *testing.T) {
	tmpDir := t.TempDir()
	mdContent := "```bash {\"stage\":\"stage1\"}\necho one\n```\n```bash {\"stage\":\"stage2\"}\necho two\n```\n"
	mdFile := path.Join(tmpDir, "test.md")
	assert.NoError(t, os.WriteFile(mdFile, []byte(mdContent), 0o644))

	run := func(start config.Location, bps ...config.Location) []string {
		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1, View: "mock",
			StartFromLocation: start, Breakpoints: bps}
		assert.NoError(t, RunMD(cfg, mdFile))
		return UnreachedBreakpoints(cfg)
	}

	t.Run("reachable breakpoints give no warning", func(t *testing.T) {
		assert.Empty(t, run(config.Location{}, config.Location{Stage: "stage1"}, config.Location{File: "test", Stage: "stage2", Chunk: "0"}))
	})
	t.Run("unknown stage", func(t *testing.T) {
		w := run(config.Location{}, config.Location{Stage: "stge1"})
		assert.Equal(t, []string{"breakpoint 'stge1' is never reached: unknown stage 'stge1'"}, w)
	})
	t.Run("stage before the start stage", func(t *testing.T) {
		w := run(config.Location{Stage: "stage2"}, config.Location{Stage: "stage1"})
		assert.Len(t, w, 1)
		assert.Contains(t, w[0], "'stage1' is never reached")
		assert.Contains(t, w[0], "before the --start-from stage")
	})
	t.Run("no such file", func(t *testing.T) {
		w := run(config.Location{}, config.Location{File: "nope", Stage: "stage1"})
		assert.Equal(t, []string{"breakpoint 'nope@stage1' is never reached: no markdown file matching 'nope' is run"}, w)
	})
}
