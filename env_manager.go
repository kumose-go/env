// Copyright (C) Kumo inc. and its affiliates.
// Author: Jeff.li lijippy@163.com
// All rights reserved.
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published
// by the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package env

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var SystemEnv map[string]int = make(map[string]int)

var InnerComponentEnv map[string]int = make(map[string]int)

const (
	ENV_CTIME_KEY = "ENV_CTIME"
)

// EnvFragment represents a single environment fragment loaded from a file.
type EnvFragment struct {
	Name     string            `yaml:"name"`
	Priority int               `yaml:"priority,omitempty"`
	Env      map[string]string `yaml:"env,omitempty"`
	Script   []Script          `yaml:"script,omitempty"`
	Source   string            // file from which this fragment was loaded
}

// Script represents a shell script snippet in the environment fragment.
type Script struct {
	Sh   string `yaml:"sh"`   // shell type: bash, zsh, powershell
	Data string `yaml:"data"` // script content
}

// EnvManager manages multiple environment fragments and merged result.
type EnvManager struct {
	// fragments maintains the order of fragments as loaded.
	fragments []*EnvFragment
	// merged contains the final merged key/value environment.
	merged map[string]string
	// keySource maps environment keys to the fragment and file that defined them.
	keySources map[string][]string
	sorted     bool
	ctime      time.Time
}

// validateFragment checks fragment priority according to its type.
func validateFragment(frag *EnvFragment) error {
	if frag.Name == "" {
		return fmt.Errorf("fragment must have a name")
	}
	switch {
	case SystemEnv[frag.Name] > 0: // builtin system fragment
		if frag.Priority > 19 {
			return fmt.Errorf("system fragment %s priority must be 0-19, got %d", frag.Name, frag.Priority)
		}
	case InnerComponentEnv[frag.Name] > 0: // internal component
		if frag.Priority < 20 || frag.Priority > 99 {
			return fmt.Errorf("internal component %s priority must be 20-99, got %d", frag.Name, frag.Priority)
		}
	default: // custom fragment
		if frag.Priority < 100 {
			return fmt.Errorf("custom fragment %s priority must >=100, got %d", frag.Name, frag.Priority)
		}
	}
	return nil
}

// FeedFile reads a YAML file containing one or more EnvFragments
// and adds them to the manager, validating priorities.
func (e *EnvManager) FeedFile(fpath string) error {
	data, err := os.ReadFile(fpath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", fpath, err)
	}

	// support multiple documents in one YAML file
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var frag EnvFragment
		if err := dec.Decode(&frag); err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("failed to parse YAML in %s: %w", fpath, err)
		}

		frag.Source = fpath // track which file this fragment came from

		if err := validateFragment(&frag); err != nil {
			return fmt.Errorf("validation failed for fragment %s in %s: %w", frag.Name, fpath, err)
		}

		e.fragments = append(e.fragments, &frag)
	}

	return nil
}

// FeedDir loads all YAML files from a directory. Non-YAML files are skipped.
func (e *EnvManager) FeedDir(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue // skip non-YAML files
		}

		fpath := filepath.Join(dir, name)
		if err := e.FeedFile(fpath); err != nil {
			return err
		}
	}

	return nil
}

func (e *EnvManager) SortAndMerge() {
	if e.merged == nil {
		e.merged = make(map[string]string)
	}
	// key -> slice of source fragment names
	e.keySources = make(map[string][]string)

	// Sort fragments by Priority ascending
	sort.SliceStable(e.fragments, func(i, j int) bool {
		return e.fragments[i].Priority < e.fragments[j].Priority
	})

	// Merge
	for _, frag := range e.fragments {
		for k, v := range frag.Env {
			e.merged[k] = v
			e.keySources[k] = append(e.keySources[k], frag.Name)
		}
	}

	// Optional: attach sources info to fragments for debugging / search
	for _, frag := range e.fragments {
		for k := range frag.Env {
			// You could keep a map in EnvFragment: map[key] -> source frag name
			// or just rely on keySources globally
			_ = e.keySources[k] // for potential future search/debug
		}
	}
	e.sorted = true
	e.ctime = time.Now()
}

func (e *EnvManager) BuildBash(dst string) error {
	if !e.sorted {
		return fmt.Errorf("not build complete yet")
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "# Env generated at %s\n", e.ctime.Format(time.RFC3339))
	fmt.Fprintf(f, "export ENV_CTIME=\"%s\"\n\n", e.ctime.Format(time.RFC3339))
	for _, frag := range e.fragments {
		fmt.Fprintf(f, "# --- Fragment: %s ---\n", frag.Name)
		for k, v := range frag.Env {
			fmt.Fprintf(f, "export %s=\"%s\"\n", k, v)
		}
		for _, sc := range frag.Script {
			if sc.Sh == "bash" {
				fmt.Fprintln(f, sc.Data)
			}
		}
		fmt.Fprintln(f)
	}
	return nil
}

// BuildZsh generates a Zsh environment file from the loaded fragments.
// Only scripts with Sh == "zsh" will be appended.
func (e *EnvManager) BuildZsh(dst string) error {
	if !e.sorted {
		return fmt.Errorf("not build complete yet")
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# Env generated at %s\n", e.ctime.Format(time.RFC3339))
	fmt.Fprintf(f, "export ENV_CTIME=\"%s\"\n", e.ctime.Format(time.RFC3339))
	for _, frag := range e.fragments {
		// Write fragment header
		if frag.Name != "" {
			fmt.Fprintf(f, "# --- Fragment: %s ---\n", frag.Name)
		}

		// Write environment variables
		for k, v := range frag.Env {
			fmt.Fprintf(f, "export %s=\"%s\"\n", k, v)
		}

		// Write Zsh scripts
		for _, sc := range frag.Script {
			if sc.Sh == "zsh" {
				fmt.Fprintln(f, sc.Data)
			}
		}

		// Separate fragments with a blank line
		fmt.Fprintln(f)
	}

	return nil
}

// BuildPsh generates a PowerShell environment file from the loaded fragments.
// Only scripts with Sh == "pw" will be appended.
func (e *EnvManager) BuildPsh(dst string) error {
	if !e.sorted {
		return fmt.Errorf("not build complete yet")
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, `$Env:ENV_CTIME = "%s"`+"\n", e.ctime.Format(time.RFC3339))
	for _, frag := range e.fragments {
		// Write fragment header
		if frag.Name != "" {
			fmt.Fprintf(f, "# --- Fragment: %s ---\n", frag.Name)
		}

		// Write environment variables
		for k, v := range frag.Env {
			fmt.Fprintf(f, `$Env:%s = "%s"`+"\n", k, v)
		}

		// Write PowerShell scripts
		for _, sc := range frag.Script {
			if sc.Sh == "pw" {
				fmt.Fprintln(f, sc.Data)
			}
		}

		// Separate fragments with a blank line
		fmt.Fprintln(f)
	}

	return nil
}

// SearchResult holds a single search result
type SearchResult struct {
	FragmentName string // fragment name
	Key          string // env key
	Value        string // env value
}

// Search looks for the given pattern in all fragments' env keys and values.
// Returns all matches with fragment information.
func (e *EnvManager) Search(pattern string) ([]SearchResult, error) {
	var re *regexp.Regexp
	var err error
	if !e.sorted {
		return nil, fmt.Errorf("not build complete yet")
	}
	// try compile as regex
	re, err = regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %v", err)
	}

	var results []SearchResult
	for _, frag := range e.fragments {
		for k, v := range frag.Env {
			if re.MatchString(k) || re.MatchString(v) {
				results = append(results, SearchResult{
					FragmentName: frag.Name,
					Key:          k,
					Value:        v,
				})
			}
		}

		// also search inside scripts
		for _, sc := range frag.Script {
			if re.MatchString(sc.Data) {
				results = append(results, SearchResult{
					FragmentName: frag.Name,
					Key:          fmt.Sprintf("script[%s]", sc.Sh),
					Value:        sc.Data,
				})
			}
		}
	}
	return results, nil
}

func ExampleEnvYaml(dst string) error {

	sample := `# Example env fragment
name: sample_service
priority: 100
env:
  SERVICE_PORT: "8080"
  SERVICE_HOST: "0.0.0.0"
script:
  - sh: bash
    data: |
      if [ -z "$SERVICE_URL" ]; then
        export SERVICE_URL="http://$SERVICE_HOST:$SERVICE_PORT"
        echo "Bash: Service URL set to $SERVICE_URL"
      fi
  - sh: zsh
    data: |
      if [[ -z "$SERVICE_URL" ]]; then
        export SERVICE_URL="http://$SERVICE_HOST:$SERVICE_PORT"
        echo "Zsh: Service URL set to $SERVICE_URL"
      fi
  - sh: pwsh
    data: |
      if (-not $env:SERVICE_URL) {
        $env:SERVICE_URL = "http://$env:SERVICE_HOST:$env:SERVICE_PORT"
        Write-Host "PowerShell: Service URL set to $env:SERVICE_URL"
      }
`

	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create example file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(sample); err != nil {
		return fmt.Errorf("failed to write example file: %v", err)
	}

	return nil
}

// WriteMeta writes the EnvManager's ctime to a metadata file in RFC3339 format.
func (e *EnvManager) WriteMeta(dst string) error {
	if !e.sorted {
		return fmt.Errorf("not gen yet")
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(e.ctime.Format(time.RFC3339))
	return err
}

// ReadEnvTime reads the ctime from a metadata file.
func ReadEnvTime(dst string) (time.Time, error) {
	data, err := os.ReadFile(dst)
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse time: %w", err)
	}
	return t, nil
}

// SaveAllYaml saves the EnvManager's fragments, sorted flag, and ctime to a YAML file.
// merged and keySources are not saved since they are runtime-generated.
func (e *EnvManager) SaveAllYaml(path string) error {
	type dumpStruct struct {
		Sorted    bool           `yaml:"sorted"`
		CTime     string         `yaml:"ctime"`
		Fragments []*EnvFragment `yaml:"fragments"`
	}

	d := dumpStruct{
		Sorted:    e.sorted,
		CTime:     e.ctime.Format(time.RFC3339),
		Fragments: e.fragments,
	}

	data, err := yaml.Marshal(&d)
	if err != nil {
		return fmt.Errorf("failed to marshal EnvManager to YAML: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write YAML file %s: %w", path, err)
	}
	return nil
}

// LoadAllYaml loads the EnvManager from a YAML file saved by SaveAllYaml.
// After loading, it automatically calls SortAndMerge() to rebuild merged and keySources.
func (e *EnvManager) LoadAllYaml(path string) error {
	type dumpStruct struct {
		Sorted    bool           `yaml:"sorted"`
		CTime     string         `yaml:"ctime"`
		Fragments []*EnvFragment `yaml:"fragments"`
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read YAML file %s: %w", path, err)
	}

	var d dumpStruct
	if err := yaml.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", err)
	}

	e.fragments = d.Fragments
	e.sorted = d.Sorted
	if d.CTime != "" {
		if t, err := time.Parse(time.RFC3339, d.CTime); err == nil {
			e.ctime = t
		}
	}

	// rebuild merged and keySources
	e.SortAndMerge()
	return nil
}
