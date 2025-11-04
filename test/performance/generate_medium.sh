#!/bin/bash
# Script to generate medium_scale.toml (100 groups, 5 commands each)

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
cat > "$SCRIPT_DIR/medium_scale.toml" << 'EOF'
# Performance Test - Medium Scale Configuration
# 100 groups with 5 commands each (total: 500 commands)
version = "1.0"

[global]
timeout = 30
log_level = "info"
env_allowed = ["PATH", "HOME", "USER", "PWD", "SHELL", "TERM"]

EOF

# Generate 100 groups with 5 commands each
for group_num in $(seq -w 1 100); do
    cat >> /home/issei/git/dryrun-debug-json-output-09/test/performance/medium_scale.toml << EOF
# Group ${group_num}
[[groups]]
name = "group_${group_num}"
description = "Performance test group ${group_num}"

[[groups.commands]]
name = "cmd_1"
description = "Echo command for group ${group_num}"
cmd = "echo"
args = ["Performance test group ${group_num} command 1"]

[[groups.commands]]
name = "cmd_2"
description = "Date command for group ${group_num}"
cmd = "date"
args = ["+%Y-%m-%d %H:%M:%S"]

[[groups.commands]]
name = "cmd_3"
description = "Whoami command for group ${group_num}"
cmd = "whoami"
args = []

[[groups.commands]]
name = "cmd_4"
description = "Pwd command for group ${group_num}"
cmd = "pwd"
args = []

[[groups.commands]]
name = "cmd_5"
description = "Env count command for group ${group_num}"
cmd = "env"
args = []

EOF
done

echo "Generated medium_scale.toml with 100 groups and 500 commands"
