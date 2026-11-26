// Package runner is responsible for orchestrating the execution of parsed
// markdown chunks. It manages the execution flow, including stages, parallel
// execution, dependencies, and teardown logic.
package runner

import (
	"errors"
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

// RunFiles executes the selected markdown files in order.
func RunFiles(cfg *config.Config, files []string) error {
	if len(cfg.Breakpoints) == 0 {
		for _, file := range files {
			ui := view.NewView(cfg.View)
			ui.StartFile(file)
			stages, err := extractStages(cfg, ui, file)
			if err != nil {
				ui.EndFile(file, err)
				return err
			}
			if err := runStages(cfg, ui, file, stages); err != nil {
				return err
			}
		}
		return nil
	}

	fileStages := make([][]*stage.Stage, len(files))
	fileViews := make([]view.RunnerView, len(files))
	for i, file := range files {
		ui := view.NewView(cfg.View)
		stages, err := extractStages(cfg, ui, file)
		if err != nil {
			return err
		}
		fileStages[i] = stages
		fileViews[i] = ui
	}
	if err := validateBreakpoints(cfg, files, fileStages); err != nil {
		return err
	}
	for i, file := range files {
		ui := fileViews[i]
		ui.StartFile(file)
		if i > 0 {
			// Earlier commands may have rewritten this file since preflight.
			stages, err := extractStages(cfg, ui, file)
			if err != nil {
				ui.EndFile(file, err)
				return err
			}
			fileStages[i] = stages
			if len(cfg.Breakpoints) > 0 {
				if err := validateBreakpoints(cfg, files[i:], fileStages[i:]); err != nil {
					ui.EndFile(file, err)
					return err
				}
			}
		}
		if err := runStages(cfg, ui, file, fileStages[i]); err != nil {
			return err
		}
	}
	return nil
}

// RunMD executes a single markdown file.
func RunMD(cfg *config.Config, file string) error {
	return RunFiles(cfg, []string{file})
}

func extractStages(cfg *config.Config, ui view.RunnerView, file string) ([]*stage.Stage, error) {
	ctx := &runnercontext.Context{Cfg: cfg, RView: ui}
	return parser.ExtractStages(ctx, path.Base(file), path.Dir(file))
}

func runStages(cfg *config.Config, ui view.RunnerView, file string, stages []*stage.Stage) error {
	tmpDirs := make(map[string]string)
	var terminatingError error
	markdownDir := path.Dir(file)
	fileName := path.Base(file)
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

// BreakpointError reports all CLI breakpoints that execution cannot reach.
type BreakpointError struct{ error }

// validateBreakpoints follows execution order without consuming the CLI locations.
func validateBreakpoints(cfg *config.Config, files []string, fileStages [][]*stage.Stage) error {
	type breakpointStatus struct {
		reason       string
		matchedStage bool
		resolved     bool
	}
	statuses := make([]breakpointStatus, len(cfg.Breakpoints))
	for i := range statuses {
		statuses[i].reason = "no such file in the selected markdown files"
		if cfg.Breakpoints[i].File == "" {
			statuses[i].reason = "unknown stage"
		}
	}

	start := config.Config{StartFromLocation: cfg.StartFromLocation}
	var startErr error
	for i, file := range files {
		for j, bp := range cfg.Breakpoints {
			if matchesFile(file, bp.File) && !statuses[j].matchedStage {
				statuses[j].reason = "unknown stage"
			}
		}
		for _, currentStage := range fileStages[i] {
			stageCopy := *currentStage
			skip, err := applyStartFrom(&start, file, &stageCopy)
			if err != nil {
				startErr = err
				// The start chunk is invalid, but the stage and chunk names
				// in the pending breakpoints can still be checked.
				start.StartFromLocation = config.Location{}
			}
			for j, bp := range cfg.Breakpoints {
				status := &statuses[j]
				if status.resolved || bp.Stage != currentStage.Name || !matchesFile(file, bp.File) {
					continue
				}
				status.matchedStage = true
				if skip {
					status.reason = "before the start stage"
					continue
				}
				status.resolved = true
				status.reason = ""
				if bp.Chunk == "" {
					continue
				}
				if _, err := findChunkByIdOrIndex(currentStage, bp.Chunk); err != nil {
					status.reason = fmt.Sprintf("unknown chunk: %v", err)
				} else if stageCopy.SkipsChunk(bp.Chunk, stageCopy.StartFromChunk) {
					status.reason = "before the start chunk"
				}
			}
		}
	}

	var diagnostics []error
	for i, status := range statuses {
		if status.reason == "" {
			continue
		}
		bp := cfg.Breakpoints[i]
		location := bp.Stage
		if bp.File != "" {
			location = bp.File + "@" + location
		}
		if bp.Chunk != "" {
			location += "/" + bp.Chunk
		}
		diagnostics = append(diagnostics, fmt.Errorf("unreachable breakpoint %q: %s", location, status.reason))
	}
	if len(diagnostics) == 0 {
		return startErr
	}
	return &BreakpointError{errors.Join(append([]error{startErr}, diagnostics...)...)}
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
