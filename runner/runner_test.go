package runner

import (
	"os"
	"path"
	"testing"

	"github.com/arkmq-org/markdown-runner/chunk"
	"github.com/arkmq-org/markdown-runner/config"
	"github.com/arkmq-org/markdown-runner/stage"
	"github.com/pterm/pterm"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	pterm.DisableOutput()
	code := m.Run()
	pterm.EnableOutput()
	os.Exit(code)
}

func TestRunMD(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {"stage":"test"}
echo "hello"
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.NoError(t, err, "Unexpected error")
	})

	t.Run("teardown", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {"stage":"main", "id":"main-chunk"}
exit 1
` + "```" + `

` + "```" + `bash {"stage":"teardown", "requires":"main/main-chunk"}
echo "teardown"
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.Error(t, err, "Expected an error, but got none")
	})

	t.Run("update file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, UpdateFile: true, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {"stage":"test"}
echo "hello"
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.NoError(t, err, "Unexpected error")

		updatedContent, err := os.ReadFile(mdFile)
		assert.NoError(t, err, "Failed to read updated file")
		expectedContent := `
` + "```" + `bash {"stage":"test"}
echo "hello"
` + "```" + `
` + "```" + `shell markdown_runner
hello
` + "```" + `
`
		assert.Equal(t, expectedContent, string(updatedContent), "File content is not as expected")
	})

	t.Run("start from", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, StartFromLocation: config.Location{Stage: "stage2"}, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {"stage":"stage1"}
echo "should not run"
` + "```" + `
` + "```" + `bash {"stage":"stage2"}
echo "should run"
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.NoError(t, err, "Unexpected error")
	})

	t.Run("start from chunk", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, StartFromLocation: config.Location{Stage: "stage2", Chunk: "second"}, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {"stage":"stage1", "runtime":"bash"}
touch ` + path.Join(tmpDir, "stage1") + `
` + "```" + `
` + "```" + `bash {"stage":"stage2", "id":"first", "runtime":"bash"}
touch ` + path.Join(tmpDir, "first") + `
` + "```" + `
` + "```" + `bash {"stage":"stage2", "id":"second", "runtime":"bash"}
touch ` + path.Join(tmpDir, "second") + `
` + "```" + `
` + "```" + `bash {"stage":"stage3", "runtime":"bash"}
touch ` + path.Join(tmpDir, "stage3") + `
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.NoError(t, err, "Unexpected error")

		for name, expected := range map[string]bool{"stage1": false, "first": false, "second": true, "stage3": true} {
			_, statErr := os.Stat(path.Join(tmpDir, name))
			assert.Equal(t, expected, statErr == nil, "unexpected execution state for chunk %s", name)
		}
	})

	t.Run("start from unknown chunk", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, StartFromLocation: config.Location{Stage: "stage1", Chunk: "missing"}, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {"stage":"stage1"}
echo "hello"
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.Error(t, err, "Expected an error for an unknown chunk")
	})

	t.Run("no stages", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1}
		mdContent := `
# No Stages Here
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.NoError(t, err, "Unexpected error")
	})

	t.Run("parser error", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)

		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {stage}
echo "bad json"
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.Error(t, err, "Expected a parser error")
	})

	t.Run("teardown with dependency", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test")
		assert.NoError(t, err, "Failed to create temp dir")
		defer os.RemoveAll(tmpDir)
		outputFile := path.Join(tmpDir, "teardown_check")

		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1}
		mdContent := `
` + "```" + `bash {"stage":"main", "id":"main-chunk"}
echo "main chunk executed"
` + "```" + `

` + "```" + `bash {"stage":"teardown", "requires":"main/main-chunk", "runtime":"bash"}
echo "teardown executed" > ` + outputFile + `
` + "```" + `
`
		mdFile := path.Join(tmpDir, "test.md")
		err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
		assert.NoError(t, err, "Failed to write to temp file")

		err = RunMD(cfg, mdFile)
		assert.NoError(t, err, "Expected no error")

		_, err = os.Stat(outputFile)
		assert.NoError(t, err, "Expected teardown chunk to be executed")
	})
}

func TestApplyBreakpoints(t *testing.T) {
	newStage := func(name string) *stage.Stage {
		return &stage.Stage{
			Name: name,
			Chunks: []*chunk.ExecutableChunk{
				{Id: "first", Stage: name},
				{Id: "second", Stage: name},
			},
		}
	}

	t.Run("a later stage breakpoint prompts again after continue", func(t *testing.T) {
		cfg := &config.Config{Breakpoints: []config.Location{{Stage: "s1"}, {Stage: "s2"}}}
		assert.NoError(t, applyBreakpoints(cfg, "dir/test.md", newStage("s1")))
		assert.True(t, cfg.Interactive)
		// the user answers "continue" during s1
		cfg.Interactive = false
		assert.NoError(t, applyBreakpoints(cfg, "dir/test.md", newStage("s2")))
		assert.True(t, cfg.Interactive, "Expected the s2 breakpoint to turn prompting back on")
		assert.Empty(t, cfg.Breakpoints)
	})

	t.Run("consumes only the breakpoints of the stage", func(t *testing.T) {
		cfg := &config.Config{Breakpoints: []config.Location{
			{Stage: "s1", Chunk: "0"},
			{Stage: "s2", Chunk: "second"},
			{Stage: "s1", Chunk: "second"},
			{File: "other.md", Stage: "s1", Chunk: "1"},
		}}
		s1 := newStage("s1")
		err := applyBreakpoints(cfg, "dir/test.md", s1)
		assert.NoError(t, err)
		assert.Equal(t, []string{"0", "second"}, s1.DebugFromChunks)
		assert.False(t, cfg.Interactive)
		assert.Equal(t, []config.Location{
			{Stage: "s2", Chunk: "second"},
			{File: "other.md", Stage: "s1", Chunk: "1"},
		}, cfg.Breakpoints)

		s2 := newStage("s2")
		err = applyBreakpoints(cfg, "dir/test.md", s2)
		assert.NoError(t, err)
		assert.Equal(t, []string{"second"}, s2.DebugFromChunks)
		assert.Equal(t, []config.Location{{File: "other.md", Stage: "s1", Chunk: "1"}}, cfg.Breakpoints)
	})

	t.Run("stage breakpoint turns interactive mode on", func(t *testing.T) {
		cfg := &config.Config{Breakpoints: []config.Location{{File: "test", Stage: "s1"}}}
		s1 := newStage("s1")
		err := applyBreakpoints(cfg, "dir/test.md", s1)
		assert.NoError(t, err)
		assert.True(t, cfg.Interactive)
		assert.Empty(t, s1.DebugFromChunks)
		assert.Empty(t, cfg.Breakpoints)
	})

	t.Run("unknown chunk is an error", func(t *testing.T) {
		cfg := &config.Config{Breakpoints: []config.Location{{Stage: "s1", Chunk: "0"}, {Stage: "s1", Chunk: "2"}}}
		err := applyBreakpoints(cfg, "dir/test.md", newStage("s1"))
		assert.Error(t, err)
	})
}

func TestApplyStartFrom(t *testing.T) {
	newStage := func(name string) *stage.Stage {
		return &stage.Stage{
			Name: name,
			Chunks: []*chunk.ExecutableChunk{
				{Id: "first", Stage: name},
				{Id: "second", Stage: name},
			},
		}
	}

	t.Run("no start-from runs every stage", func(t *testing.T) {
		cfg := &config.Config{}
		skip, err := applyStartFrom(cfg, "dir/test.md", newStage("s1"))
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("skips stages until the location is reached", func(t *testing.T) {
		cfg := &config.Config{StartFromLocation: config.Location{File: "test", Stage: "s2", Chunk: "second"}}
		skip, err := applyStartFrom(cfg, "dir/test.md", newStage("s1"))
		assert.NoError(t, err)
		assert.True(t, skip)
		skip, err = applyStartFrom(cfg, "dir/other.md", newStage("s2"))
		assert.NoError(t, err)
		assert.True(t, skip, "Expected s2 of another file to be skipped")
		s2 := newStage("s2")
		skip, err = applyStartFrom(cfg, "dir/test.md", s2)
		assert.NoError(t, err)
		assert.False(t, skip)
		assert.Equal(t, "second", s2.StartFromChunk)
		assert.Equal(t, config.Location{}, cfg.StartFromLocation)
		// Once reached, the following stages run normally
		skip, err = applyStartFrom(cfg, "dir/test.md", newStage("s1"))
		assert.NoError(t, err)
		assert.False(t, skip)
	})

	t.Run("unknown chunk is an error", func(t *testing.T) {
		cfg := &config.Config{StartFromLocation: config.Location{Stage: "s1", Chunk: "2"}}
		_, err := applyStartFrom(cfg, "dir/test.md", newStage("s1"))
		assert.Error(t, err)
	})
}

func TestRunMDWithSeveralBreakpoints(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test")
	assert.NoError(t, err, "Failed to create temp dir")
	defer os.RemoveAll(tmpDir)

	mdContent := `
` + "```" + `bash {"stage":"stage1"}
echo "one"
` + "```" + `
` + "```" + `bash {"stage":"stage2"}
echo "two"
` + "```" + `
`
	mdFile := path.Join(tmpDir, "test.md")
	err = os.WriteFile(mdFile, []byte(mdContent), 0o644)
	assert.NoError(t, err, "Failed to write to temp file")

	t.Run("every breakpoint is reached", func(t *testing.T) {
		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1, View: "mock",
			Breakpoints: []config.Location{{Stage: "stage1", Chunk: "0"}, {Stage: "stage2", Chunk: "0"}}}
		err := RunMD(cfg, mdFile)
		assert.NoError(t, err)
		assert.Empty(t, cfg.Breakpoints)
	})

	t.Run("the second breakpoint is validated", func(t *testing.T) {
		cfg := &config.Config{MarkdownDir: tmpDir, MinutesToTimeout: 1, View: "mock",
			Breakpoints: []config.Location{{Stage: "stage1", Chunk: "0"}, {Stage: "stage2", Chunk: "5"}}}
		err := RunMD(cfg, mdFile)
		assert.Error(t, err)
	})
}

func TestRunMDRejectsUnreachableBreakpoints(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "pipeline.md")
	contents := "```bash {\"stage\":\"setup\"}\ntouch \"" + path.Join(dir, "executed") + "\"\n```\n" +
		"```bash {\"stage\":\"deploy\",\"id\":\"first\"}\ntrue\n```\n" +
		"```bash {\"stage\":\"deploy\",\"id\":\"second\"}\ntrue\n```\n"
	if err := os.WriteFile(file, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		View:              "mock",
		MinutesToTimeout:  1,
		StartFromLocation: config.Location{File: "pipeline", Stage: "deploy", Chunk: "second"},
		Breakpoints: []config.Location{
			{Stage: "deplyo"},
			{Stage: "deploy", Chunk: "nope"},
			{Stage: "setup"},
			{File: "missing", Stage: "deploy"},
			{Stage: "deploy", Chunk: "first"},
			{File: "pipeline", Stage: "deploy", Chunk: "second"},
		},
	}
	err := RunMD(cfg, file)
	for _, diagnostic := range []string{
		`"deplyo": unknown stage`,
		`"deploy/nope": unknown chunk`,
		`"setup": before the start stage`,
		`"missing@deploy": no such file`,
		`"deploy/first": before the start chunk`,
	} {
		assert.ErrorContains(t, err, diagnostic)
	}
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "pipeline@deploy/second")
	}

	cfg.StartFromLocation = config.Location{}
	assert.Error(t, RunMD(cfg, file))
	assert.NoFileExists(t, path.Join(dir, "executed"))
}

func TestRunMDReportsBreakpointsWithInvalidStartChunk(t *testing.T) {
	file := path.Join(t.TempDir(), "pipeline.md")
	if err := os.WriteFile(file, []byte("```bash {\"stage\":\"deploy\"}\ntrue\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		View:              "mock",
		StartFromLocation: config.Location{Stage: "deploy", Chunk: "nope"},
		Breakpoints: []config.Location{
			{Stage: "deplyo"},
			{File: "missing", Stage: "deploy"},
		},
	}
	err := RunMD(cfg, file)
	assert.ErrorContains(t, err, "chunk with ID 'nope' not found in stage 'deploy'")
	assert.ErrorContains(t, err, `"deplyo": unknown stage`)
	assert.ErrorContains(t, err, `"missing@deploy": no such file`)
}

func TestRunFilesReadsUpdatedMarkdownWithBreakpoint(t *testing.T) {
	dir := t.TempDir()
	first, second := path.Join(dir, "a.md"), path.Join(dir, "b.md")
	oldMarker, newMarker := path.Join(dir, "OLD"), path.Join(dir, "NEW")
	for file, contents := range map[string]string{
		first:  "```bash {\"stage\":\"setup\"}\nsed -i 's/OLD/NEW/' \"" + second + "\"\n```\n",
		second: "```bash {\"stage\":\"deploy\"}\ntouch \"" + oldMarker + "\"\n```\n",
	} {
		if err := os.WriteFile(file, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{
		MarkdownDir: dir, MinutesToTimeout: 1, View: "mock",
		Breakpoints: []config.Location{{File: "b", Stage: "deploy", Chunk: "0"}},
	}
	assert.NoError(t, RunFiles(cfg, []string{first, second}))
	assert.Empty(t, cfg.Breakpoints)
	assert.FileExists(t, newMarker)
	assert.NoFileExists(t, oldMarker)
}

func TestRunFilesResolvesBreakpointCreatedByEarlierFile(t *testing.T) {
	for _, tc := range []struct {
		name, stage, id, replacement string
	}{
		{name: "stage", stage: "draft", id: "target", replacement: "deploy"},
		{name: "chunk", stage: "deploy", id: "draft", replacement: "target"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			first, second := path.Join(dir, "a.md"), path.Join(dir, "b.md")
			marker := path.Join(dir, "executed")
			for file, contents := range map[string]string{
				first:  "```bash {\"stage\":\"setup\"}\nsed -i 's/draft/" + tc.replacement + "/' \"" + second + "\"\n```\n",
				second: "```bash {\"stage\":\"" + tc.stage + "\",\"id\":\"" + tc.id + "\"}\ntouch \"" + marker + "\"\n```\n",
			} {
				if err := os.WriteFile(file, []byte(contents), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.Config{
				MarkdownDir: dir, MinutesToTimeout: 1, View: "mock",
				Breakpoints: []config.Location{{File: "b", Stage: "deploy", Chunk: "target"}},
			}
			assert.NoError(t, RunFiles(cfg, []string{first, second}))
			assert.Empty(t, cfg.Breakpoints)
			assert.FileExists(t, marker)
		})
	}
}
