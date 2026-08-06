package pullrequest

import (
	"errors"
	"fmt"
	"strings"

	"github.com/avitsrimer/bitbucket-cli/internal/common"
	"github.com/spf13/cobra"
)

// registerDescriptionFileFlag registers --description-file on cmd, binding it to
// *descriptionFile and marking it mutually exclusive with --description. Shared by
// pullrequest create/update, whose existing --description flag registration (and its own
// required-ness rules, unchanged by this flag) each command keeps doing on its own.
func registerDescriptionFileFlag(cmd *cobra.Command, descriptionFile *string) {
	cmd.Flags().StringVar(descriptionFile, "description-file", "", "Read the description of the pullrequest from <path>, or - to read it from stdin. Mutually exclusive with --description.")
	cmd.MarkFlagsMutuallyExclusive("description", "description-file")
	_ = cmd.MarkFlagFilename("description-file")
}

// resolveDescriptionBody returns the pull request description to use: descriptionFile's content
// (or cmd's stdin, via "-") when descriptionFile is set, otherwise description verbatim. A
// descriptionFile whose content is empty (after trimming) is rejected, consistent with FR-6's
// empty-body rule: --description alone legitimately clears the description (it is an optional
// field), but a --description-file pointing at an empty file is a foot-gun, not a deliberate
// choice, so it fails instead of silently producing the same result.
func resolveDescriptionBody(cmd *cobra.Command, description, descriptionFile string) (string, error) {
	if descriptionFile == "" {
		return description, nil
	}
	body, err := common.ReadBodyFromFileOrStdin(cmd, descriptionFile)
	if err != nil {
		return "", fmt.Errorf("cannot read description: %w", err)
	}
	if strings.TrimSpace(body) == "" {
		return "", errors.New("description body is empty")
	}
	return body, nil
}
