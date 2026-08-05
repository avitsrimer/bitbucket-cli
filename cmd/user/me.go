package user

import (
	"github.com/gildas/bitbucket-cli/cmd/profile"
	"github.com/gildas/go-flags"
	"github.com/go-pkgz/lgr"
	"github.com/spf13/cobra"
)

var meCmd = &cobra.Command{
	Use:     "me",
	Aliases: []string{"self"},
	Short:   "get the current authenticated user",
	Args:    cobra.NoArgs,
	RunE:    meProcess,
}

var meOptions struct {
	Emails  bool
	Columns *flags.EnumSliceFlag
}

func init() {
	Command.AddCommand(meCmd)

	meOptions.Columns = flags.NewEnumSliceFlag(columns.Columns()...)
	meCmd.Flags().BoolVar(&meOptions.Emails, "emails", false, "Display the email addresses of the current authenticated user")
	meCmd.Flags().Var(meOptions.Columns, "columns", "Comma-separated list of columns to display")
	_ = meCmd.RegisterFlagCompletionFunc(meOptions.Columns.CompletionFunc("columns"))
}

func meProcess(cmd *cobra.Command, args []string) (err error) {
	if meOptions.Emails {
		emails, emailsErr := profile.GetAll[Email](cmd.Context(), cmd, "/user/emails")
		if emailsErr != nil {
			return emailsErr
		}
		lgr.Printf("[DEBUG] displaying emails for the current authenticated user")
		return profile.Current.Print(cmd.Context(), cmd, Emails(emails))
	}

	profile, err := profile.GetProfileFromCommand(cmd.Context(), cmd)
	if err != nil {
		return err
	}

	lgr.Printf("[DEBUG] displaying current authenticated user")
	user, err := GetMe(cmd.Context(), cmd)
	if err != nil {
		return err
	}
	lgr.Printf("[DEBUG] current user retrieved")
	return profile.Print(cmd.Context(), cmd, user)
}
