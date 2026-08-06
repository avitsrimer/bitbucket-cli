package pipeline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/avitsrimer/bitbucket-cli/internal/pipeline/step"
	"github.com/avitsrimer/bitbucket-cli/internal/user"
	"github.com/gildas/go-core"
	"github.com/spf13/cobra"
)

// Pipeline represents a BitBucket pipeline run.
type Pipeline struct {
	ID                   common.UUID           `json:"uuid"`
	BuildNumber          uint64                `json:"build_number"`
	State                PipelineState         `json:"state"`
	Creator              user.User             `json:"creator"`
	Repository           Repository            `json:"repository"`
	Target               Target                `json:"target"`
	Variables            []Variable            `json:"variables,omitempty"`
	ConfigurationSources []ConfigurationSource `json:"configuration_sources,omitempty"`
	Duration             time.Duration         `json:"duration_in_seconds"`
	CreatedOn            time.Time             `json:"created_on"`
	CompletedOn          time.Time             `json:"completed_on"`
	Links                common.Links          `json:"links"`
}

// Repository is the repository reference embedded in a Pipeline.
type Repository struct {
	Type     string       `json:"type"`
	UUID     string       `json:"uuid,omitempty"`
	Name     string       `json:"name,omitempty"`
	FullName string       `json:"full_name,omitempty"`
	Links    common.Links `json:"links"`
}

// ConfigurationSource represents one of a pipeline's configuration sources.
type ConfigurationSource struct {
	Source string `json:"source"`
	URI    string `json:"uri"`
}

// Command represents this folder's command
var Command = &cobra.Command{
	Use:     "pipeline",
	Aliases: []string{"pipelines", "pipe", "pp"},
	Short:   "Manage pipelines",
	Run:     common.SubcommandRequired("Pipeline"),
}

func init() {
	Command.AddCommand(step.Command)
}

var columns = common.Columns[Pipeline]{
	{Name: "build_number", DefaultSorter: true, Compare: func(a, b Pipeline) bool {
		return a.BuildNumber < b.BuildNumber
	}},
	{Name: "uuid", DefaultSorter: false, Compare: func(a, b Pipeline) bool {
		return strings.ToLower(a.ID.String()) < strings.ToLower(b.ID.String())
	}},
	{Name: "state", DefaultSorter: false, Compare: func(a, b Pipeline) bool {
		return strings.ToLower(a.State.Name) < strings.ToLower(b.State.Name)
	}},
	{Name: "branch", DefaultSorter: false, Compare: func(a, b Pipeline) bool {
		return strings.ToLower(a.Target.GetDestination()) < strings.ToLower(b.Target.GetDestination())
	}},
	{Name: "creator", DefaultSorter: false, Compare: func(a, b Pipeline) bool {
		return strings.ToLower(a.Creator.Name) < strings.ToLower(b.Creator.Name)
	}},
	{Name: "duration", DefaultSorter: false, Compare: func(a, b Pipeline) bool {
		return a.Duration < b.Duration
	}},
	{Name: "created_on", DefaultSorter: false, Compare: func(a, b Pipeline) bool {
		return a.CreatedOn.Before(b.CreatedOn)
	}},
	{Name: "completed_on", DefaultSorter: false, Compare: func(a, b Pipeline) bool {
		if a.CompletedOn.IsZero() && b.CompletedOn.IsZero() {
			return false
		}
		if a.CompletedOn.IsZero() {
			return true
		}
		if b.CompletedOn.IsZero() {
			return false
		}
		return a.CompletedOn.Before(b.CompletedOn)
	}},
}

// GetType gets the type of this pipeline
//
// implements core.TypeCarrier
func (pipeline Pipeline) GetType() string {
	return "pipeline"
}

// GetHeaders gets the header for a table
//
// implements common.Tableable
func (pipeline Pipeline) GetHeaders(cmd *cobra.Command) []string {
	if cmd != nil && cmd.Flag("columns") != nil && cmd.Flag("columns").Changed {
		if values, err := cmd.Flags().GetStringSlice("columns"); err == nil {
			return core.Map(values, func(column string) string { return strings.ReplaceAll(column, "_", " ") })
		}
	}
	return []string{"Build Number", "State", "Branch", "Creator", "Duration", "Created On"}
}

// GetRow gets the row for a table
//
// implements common.Tableable
func (pipeline Pipeline) GetRow(headers []string) []string {
	row := make([]string, 0, len(headers))

	for _, header := range headers {
		switch common.NormalizeColumnKey(header) {
		case "build_number":
			row = append(row, strconv.FormatUint(pipeline.BuildNumber, 10))
		case "uuid":
			row = append(row, pipeline.ID.String())
		case "state":
			row = append(row, pipeline.State.String())
		case "branch":
			row = append(row, pipeline.Target.GetDestination())
		case "creator":
			row = append(row, pipeline.Creator.Name)
		case "duration":
			row = append(row, pipeline.Duration.String())
		case "created_on":
			row = append(row, pipeline.createdOnCell())
		case "completed_on":
			row = append(row, pipeline.completedOnCell())
		default:
			row = append(row, " ")
		}
	}
	return row
}

// completedOnCell returns CompletedOn formatted with common.TableTimeFormat, or " " when it is
// zero (a pipeline still in progress has no completion time).
func (pipeline Pipeline) completedOnCell() string {
	if pipeline.CompletedOn.IsZero() {
		return " "
	}
	return pipeline.CompletedOn.Format(common.TableTimeFormat)
}

// createdOnCell returns CreatedOn formatted with common.TableTimeFormat, or " " when it is zero.
func (pipeline Pipeline) createdOnCell() string {
	if pipeline.CreatedOn.IsZero() {
		return " "
	}
	return pipeline.CreatedOn.Format(common.TableTimeFormat)
}

// String gets a string representation of this pipeline
//
// implements fmt.Stringer
func (pipeline Pipeline) String() string {
	return fmt.Sprintf("#%d", pipeline.BuildNumber)
}

// MarshalJSON implements the json.Marshaler interface.
func (pipeline Pipeline) MarshalJSON() (data []byte, err error) {
	type surrogate Pipeline

	var completedOn string
	if !pipeline.CompletedOn.IsZero() {
		completedOn = pipeline.CompletedOn.Format(common.JSONTimeFormat)
	}

	data, err = json.Marshal(struct {
		Type string `json:"type"`
		surrogate
		CreatedOn         string `json:"created_on"`
		CompletedOn       string `json:"completed_on,omitempty"`
		DurationInSeconds uint64 `json:"duration_in_seconds"`
	}{
		Type:              pipeline.GetType(),
		surrogate:         surrogate(pipeline),
		CreatedOn:         pipeline.CreatedOn.Format(common.JSONTimeFormat),
		CompletedOn:       completedOn,
		DurationInSeconds: uint64(pipeline.Duration.Seconds()),
	})
	if err != nil {
		return nil, fmt.Errorf("cannot marshal pipeline to json: %w", err)
	}
	return data, nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (pipeline *Pipeline) UnmarshalJSON(data []byte) (err error) {
	type surrogate Pipeline
	var inner struct {
		Type string `json:"type"`
		surrogate
		DurationInSeconds uint64 `json:"duration_in_seconds"`
	}
	if err = json.Unmarshal(data, &inner); err != nil {
		return fmt.Errorf("cannot unmarshal pipeline: %w", err)
	}
	if inner.Type != "" && inner.Type != (Pipeline{}).GetType() {
		return fmt.Errorf("cannot unmarshal pipeline: invalid type %s, expected %s", inner.Type, (Pipeline{}).GetType())
	}
	*pipeline = Pipeline(inner.surrogate)
	pipeline.Duration = time.Duration(inner.DurationInSeconds) * time.Second //nolint:gosec // duration_in_seconds is a pipeline runtime in seconds from BitBucket, far below the ~292 billion year range where this conversion could overflow
	return nil
}
