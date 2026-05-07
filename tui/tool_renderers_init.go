package tui

func init() {
	RegisterToolRenderer("read_file", readFileRenderer{})
	RegisterToolRenderer("list_files", listFilesRenderer{})
	RegisterToolRenderer("search_text", searchTextRenderer{})
	RegisterToolRenderer("run_shell", runShellRenderer{})
	RegisterToolRenderer("write_file", writeFileRenderer{})
	RegisterToolRenderer("replace_in_file", replaceInFileRenderer{})
	RegisterToolRenderer("apply_patch", applyPatchRenderer{})
	RegisterToolRenderer("update_plan", updatePlanRenderer{})
	RegisterToolRenderer("web_search", webSearchRenderer{})
	RegisterToolRenderer("web_fetch", webFetchRenderer{})
}
