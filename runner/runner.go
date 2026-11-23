// Package runner is responsible for orchestrating the execution of parsed
// markdown chunks. It manages the execution flow, including stages, parallel
// execution, dependencies, and teardown logic.
package runner

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/arkmq-org/markdown-runner/chunk"
	"github.com/arkmq-org/markdown-runner/config"
	"github.com/arkmq-org/markdown-runner/parser"
	"github.com/arkmq-org/markdown-runner/runnercontext"
	"github.com/arkmq-org/markdown-runner/stage"
	"github.com/arkmq-org/markdown-runner/view"
)

// RunMD orchestrates the entire execution process for a single markdown file.
// It parses the file to get the stages, then iterates through them, executing
// the chunks in order. It handles parallel execution, dependencies, and teardown
// stages. If the configuration is set, it will also update the markdown file
// with the output of the executed chunks.
//
// file is the name of the markdown file to execute.
// It returns an error if any chunk fails and is not part of a teardown stage.
func RunMD(cfg *config.Config, file string) error {
	var tmpDirs map[string]string = make(map[string]string)
	var terminatingError error
	markdownDir := path.Dir(file)
	fileName := path.Base(file)
	ui := view.NewView(cfg.View)
	ctx := &runnercontext.Context{
		Cfg:   cfg,
		RView: ui,
	}

	ui.StartFile(file)

	stages, err := parser.ExtractStages(ctx, fileName, markdownDir)
	if err != nil {
		ui.EndFile(file, err)
		return err
	}
	if len(stages) == 0 {
		return nil
	}

	for _, currentStage := range stages {
		// Handle start-from flag: skip everything before the specified stage/chunk
		if skip, err := applyStartFrom(cfg, file, currentStage); err != nil {
			ui.EndFile(file, err)
			return err
		} else if skip {
			continue
		}

		// Handle break-at flags: start interactive mode when we reach the specified stages/chunks
		if err := applyBreakpoints(cfg, file, currentStage); err != nil {
			ui.EndFile(file, err)
			return err
		}

		ui.StartStage(currentStage.Name, len(currentStage.Chunks), cfg.Verbose)

		var err error

		err = currentStage.Execute(stages, tmpDirs, terminatingError)
		if err != nil {
			terminatingError = err
		}

	}

	if cfg.UpdateFile && terminatingError == nil {
		terminatingError = parser.UpdateChunkOutput(fileName, markdownDir, stages)
		if terminatingError == nil {
			os.Rename(path.Join(markdownDir, fileName+".out"), path.Join(markdownDir, fileName))
		}
	}

	for _, tmpDir := range tmpDirs {
		os.RemoveAll(tmpDir)
	}

	ui.EndFile(file, terminatingError)
	return terminatingError
}

// CheckBreakpoints validates the --break-at breakpoints before anything runs.
// It parses every file that is about to be run and replays the --start-from
// logic to know which stages will be executed. It returns one warning per
// breakpoint that can never be reached, naming it and saying why (unknown
// stage, stage before the --start-from stage, no such file). A breakpoint on a
// chunk that doesn't exist in a stage that will run is an error, as it already
// was when that stage was reached.
// Files that can't be parsed are left to the run, which reports the error.
func CheckBreakpoints(cfg *config.Config, files []string) ([]string, error) {
	if len(cfg.Breakpoints) == 0 {
		return nil, nil
	}
	type visit struct {
		file, stage string
		skipped     bool
		s           *stage.Stage
	}
	var visits []visit
	sim := *cfg // simulate --start-from without consuming the real one
	for _, file := range files {
		ctx := &runnercontext.Context{Cfg: &sim, RView: view.NewView("mock")}
		stages, err := parser.ExtractStages(ctx, path.Base(file), path.Dir(file))
		if err != nil {
			return nil, nil
		}
		for _, st := range stages {
			skip, err := applyStartFrom(&sim, file, st)
			if err != nil {
				return nil, nil
			}
			visits = append(visits, visit{file, st.Name, skip, st})
		}
	}
	var warnings []string
	for _, bp := range cfg.Breakpoints {
		fileSeen, skipped, found := false, false, false
		for _, v := range visits {
			if !matchesFile(v.file, bp.File) {
				continue
			}
			fileSeen = true
			if v.stage != bp.Stage {
				continue
			}
			if v.skipped {
				skipped = true
				continue
			}
			// At run time a breakpoint is used up by the first stage that
			// matches and isn't skipped: validate it against that one only.
			found = true
			if bp.Chunk != "" {
				if _, err := findChunkByIdOrIndex(v.s, bp.Chunk); err != nil {
					return nil, err
				}
			}
			break
		}
		var why string
		switch {
		case found:
			continue
		case !fileSeen:
			why = fmt.Sprintf("no markdown file matching '%s' is run", bp.File)
		case skipped:
			why = fmt.Sprintf("stage '%s' comes before the --start-from stage and is skipped", bp.Stage)
		default:
			why = fmt.Sprintf("unknown stage '%s'", bp.Stage)
		}
		warnings = append(warnings, fmt.Sprintf("breakpoint '%s' is never reached: %s", bp, why))
	}
	return warnings, nil
}

// matchesFile reports whether the markdown file being executed matches the
// file part of a file@stage specification. An empty specification matches any file.
// Check various forms: full path, basename, basename without extension.
func matchesFile(file string, spec string) bool {
	if spec == "" {
		return true
	}
	basename := path.Base(file)
	nameWithoutExt := strings.TrimSuffix(basename, path.Ext(basename))
	return file == spec ||
		basename == spec ||
		nameWithoutExt == spec ||
		strings.HasSuffix(file, spec)
}

// applyStartFrom tells whether the given stage of the given file precedes the
// --start-from location and must therefore be skipped. Once the location is
// reached, it is cleared so that the following stages run normally and, when a
// chunk is given, it is handed over to the stage so that it skips the chunks
// preceding it.
// It returns an error if the targeted chunk doesn't exist in the stage.
func applyStartFrom(cfg *config.Config, file string, currentStage *stage.Stage) (bool, error) {
	loc := cfg.StartFromLocation
	if loc.Stage == "" {
		return false, nil
	}
	if loc.Stage != currentStage.Name || !matchesFile(file, loc.File) {
		return true, nil
	}
	if loc.Chunk != "" {
		// Specific chunk requested - validate it exists
		if _, err := findChunkByIdOrIndex(currentStage, loc.Chunk); err != nil {
			return false, err
		}
		currentStage.StartFromChunk = loc.Chunk
	}
	cfg.StartFromLocation = config.Location{}
	return false, nil
}

// applyBreakpoints consumes the --break-at breakpoints targeting the given
// stage of the given file. A stage breakpoint turns the interactive mode on
// right away, a chunk breakpoint is handed over to the stage so that it turns
// the interactive mode on when reaching that chunk. Each breakpoint is
// consumed once.
// It returns an error if a targeted chunk doesn't exist in the stage.
func applyBreakpoints(cfg *config.Config, file string, currentStage *stage.Stage) error {
	var remaining []config.Location
	for _, bp := range cfg.Breakpoints {
		if bp.Stage != currentStage.Name || !matchesFile(file, bp.File) {
			remaining = append(remaining, bp)
			continue
		}
		if bp.Chunk == "" {
			// Debug entire stage
			cfg.Interactive = true
			continue
		}
		// Specific chunk requested - validate it exists
		if _, err := findChunkByIdOrIndex(currentStage, bp.Chunk); err != nil {
			return err
		}
		currentStage.DebugFromChunks = append(currentStage.DebugFromChunks, bp.Chunk)
	}
	cfg.Breakpoints = remaining
	return nil
}

// findChunkByIdOrIndex finds a chunk in a stage by either its ID or by index (0-based)
func findChunkByIdOrIndex(stage *stage.Stage, identifier string) (*chunk.ExecutableChunk, error) {
	// Try to parse as integer index first
	if index, err := strconv.Atoi(identifier); err == nil {
		if index < 0 || index >= len(stage.Chunks) {
			return nil, fmt.Errorf("chunk index %d is out of range (0-%d) in stage '%s'",
				index, len(stage.Chunks)-1, stage.Name)
		}
		return stage.Chunks[index], nil
	}

	// Try to find by ID
	for _, chunk := range stage.Chunks {
		if chunk.Id == identifier {
			return chunk, nil
		}
	}

	return nil, fmt.Errorf("chunk with ID '%s' not found in stage '%s'", identifier, stage.Name)
}
