package pullrequest

var unapproveSpec = actionSpec{
	name:     "unapprove",
	short:    "unapprove a pullrequest by its <pullrequest-id>. If not provided, it will try to unapprove the only open pullrequest.",
	whatIf:   "Unapproving",
	errVerb:  "unapprove",
	endpoint: "approve",
}

func init() {
	Command.AddCommand(newActionCommand(unapproveSpec))
}
