package pullrequest

var approveSpec = actionSpec{
	name:     "approve",
	short:    "approve a pullrequest by its <pullrequest-id>. If not provided, it will try to approve the only open pullrequest.",
	whatIf:   "Approving",
	errVerb:  "approve",
	endpoint: "approve",
	post:     true,
}

func init() {
	Command.AddCommand(newActionCommand(approveSpec))
}
