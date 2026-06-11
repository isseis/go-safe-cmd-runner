# Task Directory Identification

When a command needs to identify the target task directory under `docs/tasks/`, use the following priority rules in order:

1. **Argument**: If `$ARGUMENTS` is non-empty, treat it as a task identifier (e.g. `0134` or `0134_implicit_system_lib_exclusion`) and resolve it to the matching directory under `docs/tasks/`. Stop with an error if no directory matches.
2. **Open file**: If a file under `docs/tasks/<task>/` is currently open in the IDE, use that task directory.
3. **Ambiguous**: If neither rule applies, stop. List all candidate task directories and ask the user to re-run with an explicit task identifier as the argument (e.g. `/mkarch 0020`).
