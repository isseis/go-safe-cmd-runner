#!/bin/bash
# Script to generate large_scale.toml (500 groups, 10 commands each)

cat > /home/issei/git/dryrun-debug-json-output-09/test/performance/large_scale.toml << 'EOF'
# Performance Test - Large Scale Configuration
# 500 groups with 10 commands each (total: 5000 commands)
version = "1.0"

[global]
timeout = 30
log_level = "info"
env_allowed = ["PATH", "HOME", "USER", "PWD", "SHELL", "TERM", "LANG", "TZ"]

EOF

# Generate 500 groups with 10 commands each
for group_num in $(seq -w 1 500); do
    cat >> /home/issei/git/dryrun-debug-json-output-09/test/performance/large_scale.toml << EOF
# Group ${group_num}
[[groups]]
name = "group_${group_num}"
description = "Performance test group ${group_num}"

[[groups.commands]]
name = "cmd_1"
description = "Echo command 1 for group ${group_num}"
cmd = "echo"
args = ["Performance test group ${group_num} command 1"]

[[groups.commands]]
name = "cmd_2"
description = "Date command 2 for group ${group_num}"
cmd = "date"
args = ["+%Y-%m-%d %H:%M:%S"]

[[groups.commands]]
name = "cmd_3"
description = "Whoami command 3 for group ${group_num}"
cmd = "whoami"
args = []

[[groups.commands]]
name = "cmd_4"
description = "Pwd command 4 for group ${group_num}"
cmd = "pwd"
args = []

[[groups.commands]]
name = "cmd_5"
description = "Env command 5 for group ${group_num}"
cmd = "env"
args = []

[[groups.commands]]
name = "cmd_6"
description = "Hostname command 6 for group ${group_num}"
cmd = "hostname"
args = []

[[groups.commands]]
name = "cmd_7"
description = "Uptime command 7 for group ${group_num}"
cmd = "uptime"
args = []

[[groups.commands]]
name = "cmd_8"
description = "Id command 8 for group ${group_num}"
cmd = "id"
args = []

[[groups.commands]]
name = "cmd_9"
description = "Uname command 9 for group ${group_num}"
cmd = "uname"
args = ["-a"]

[[groups.commands]]
name = "cmd_10"
description = "Date unix command 10 for group ${group_num}"
cmd = "date"
args = ["+%s"]

EOF
done

echo "Generated large_scale.toml with 500 groups and 5000 commands"
