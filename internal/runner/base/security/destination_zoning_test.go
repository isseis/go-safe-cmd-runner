package security

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test fixtures and helpers ---

func foreignIdent() risktypes.RunAsIdent {
	return risktypes.RunAsIdent{UID: uint32(os.Geteuid()) + 1, GID: uint32(os.Getgid()) + 1}
}

func selfIdent() risktypes.RunAsIdent {
	return risktypes.RunAsIdent{UID: uint32(os.Geteuid()), GID: uint32(os.Getgid())}
}

// zoningWorkdir returns a fresh safe-zone working directory under the test temp.
func zoningWorkdir(t *testing.T) string {
	t.Helper()
	wd := filepath.Join(tempRoot(t), "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	return wd
}

func zoningInput(workdir string, ident risktypes.RunAsIdent) ZoningInput {
	return ZoningInput{
		EffectiveWorkDir:    workdir,
		SystemCriticalPaths: []string{"/", "/usr", "/etc", "/bin", "/sbin", "/dev"},
		TrustedDirectories:  []string{workdir},
		RunAsIdent:          ident,
		OutputCriticalPathPatterns: []string{
			"/etc/shadow", "/etc/passwd", "id_rsa", ".ssh/", "private_key",
		},
		MaxOperands:    64,
		MaxSymlinkHops: MaxSymlinkDepth,
	}
}

func classify(in ZoningInput, cmd string, args ...string) LocationResult {
	return ClassifyDestinationZone(in, cmdNameSet(cmd), cmd, args)
}

func hasWriteOperand(ops []risktypes.OperandZone) bool {
	for _, oz := range ops {
		if oz.Role == risktypes.OperandRoleWrite {
			return true
		}
	}
	return false
}

// fakeFileInfo is a synthetic fs.FileInfo for injecting device/permission modes.
type fakeFileInfo struct {
	name string
	mode fs.FileMode
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

// erroringResolver resolves nothing: every lstat fails with a non-ENOENT error,
// so all operands fall to ZoneUnresolved.
func erroringResolver() *operandResolver {
	return newOperandResolver(
		func(string) (fs.FileInfo, error) { return nil, os.ErrPermission },
		func(string) (string, error) { return "", os.ErrPermission },
	)
}

// --- trust-critical ---

func TestClassifyDestinationZone_TrustCritical(t *testing.T) {
	in := zoningInput(zoningWorkdir(t), foreignIdent())

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "touch", "/usr/bin/zzz_runplan").Level,
		"write under /usr is trust-critical High")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "touch", "/etc/zzz_runplan").Level,
		"write under /etc is trust-critical High")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "rm", "/").Level,
		"/ matches by exact equality")
}

// --- ordinary ---

func TestClassifyDestinationZone_Ordinary(t *testing.T) {
	in := zoningInput(zoningWorkdir(t), foreignIdent())

	// /srv and /opt are not critical and (typically) do not exist: they fold to a
	// real parent and classify ordinary.
	assert.Equal(t, runnertypes.RiskLevelMedium, classify(in, "touch", "/srv/app/cache.dat").Level)
	assert.Equal(t, runnertypes.RiskLevelMedium, classify(in, "rm", "/opt/pkg/data").Level)
}

// --- safe-zone (Trusted Low / non-Trusted Medium) ---

func TestClassifyDestinationZone_SafeZone(t *testing.T) {
	wd := zoningWorkdir(t)

	trusted := classify(zoningInput(wd, foreignIdent()), "touch", filepath.Join(wd, "out"))
	assert.Equal(t, runnertypes.RiskLevelLow, trusted.Level, "Trusted safe-zone is Low")

	untrusted := classify(zoningInput(wd, selfIdent()), "touch", filepath.Join(wd, "out"))
	assert.Equal(t, runnertypes.RiskLevelMedium, untrusted.Level, "non-Trusted safe-zone falls back to Medium")
}

// --- safe-zone origin is only the configured work/temp dir ---

func TestSafeZoneOrigin(t *testing.T) {
	root := tempRoot(t)
	wd := filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(wd, 0o700))
	// A sibling directory (standing in for $HOME or a shared /tmp) is NOT the
	// configured origin, so a write there is ordinary, not a Low safe-zone.
	sibling := filepath.Join(root, "elsewhere")
	require.NoError(t, os.MkdirAll(sibling, 0o700))

	in := zoningInput(wd, foreignIdent())
	got := classify(in, "touch", filepath.Join(sibling, "out"))
	assert.Equal(t, runnertypes.RiskLevelMedium, got.Level,
		"a path outside the configured origin is not a Low safe-zone")
}

// --- overlap resolves to trust-critical ---

func TestSafeZoneOverlapsCritical(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())
	in.SystemCriticalPaths = append(in.SystemCriticalPaths, wd) // overlap origin with a critical path

	got := classify(in, "touch", filepath.Join(wd, "x"))
	assert.Equal(t, runnertypes.RiskLevelHigh, got.Level,
		"trust-critical takes precedence over a safe-zone it overlaps")
}

// --- unresolved asymmetry (write High / read Medium) ---

func TestUnresolvedAsymmetry(t *testing.T) {
	// Direct level mapping.
	assert.Equal(t, runnertypes.RiskLevelHigh, zoneLevel(risktypes.ZoneUnresolved, risktypes.OperandRoleWrite, false))
	assert.Equal(t, runnertypes.RiskLevelMedium, zoneLevel(risktypes.ZoneUnresolved, risktypes.OperandRoleRead, false))

	in := zoningInput(zoningWorkdir(t), foreignIdent())

	// A write whose destination cannot be resolved is High.
	rmRes := classifyDestinationZone(in, cmdNameSet("rm"), "rm", []string{"/x"}, erroringResolver())
	assert.Equal(t, runnertypes.RiskLevelHigh, rmRes.Level)
	require.Len(t, rmRes.Operands, 1)
	assert.Equal(t, risktypes.ZoneUnresolved, rmRes.Operands[0].Zone)
	assert.NotEmpty(t, rmRes.Operands[0].UnresolvedErr)

	// A copy with both operands unresolved records read=Medium and write=High.
	cpRes := classifyDestinationZone(in, cmdNameSet("cp"), "cp", []string{"/src", "/dst"}, erroringResolver())
	assert.Equal(t, runnertypes.RiskLevelHigh, cpRes.Level)
	for _, oz := range cpRes.Operands {
		assert.Equal(t, risktypes.ZoneUnresolved, oz.Zone)
		if oz.Role == risktypes.OperandRoleRead {
			assert.Equal(t, runnertypes.RiskLevelMedium, zoneLevel(oz.Zone, oz.Role, false))
		}
	}
}

// --- extraction spec-table difficult cases ---

func TestOperandExtraction_SpecTable(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	// A symlink inside the work dir that points at /etc, to exercise trailing-slash
	// dereference.
	etcLink := filepath.Join(wd, "etclink")
	require.NoError(t, os.Symlink("/etc", etcLink))

	tests := []struct {
		name      string
		cmd       string
		args      []string
		wantApply bool
		wantLevel runnertypes.RiskLevel
	}{
		{"in_place_truncate", "truncate", []string{"-s", "0", "/usr/bin/x"}, true, runnertypes.RiskLevelHigh},
		{"in_place_sed", "sed", []string{"-i", "s/a/b/", "/usr/bin/x"}, true, runnertypes.RiskLevelHigh},
		{"sed_without_i_not_applicable", "sed", []string{"s/a/b/", "/usr/bin/x"}, false, runnertypes.RiskLevelLow},
		{"ln_symbolic_to_critical", "ln", []string{"-s", "/etc/passwd", filepath.Join(wd, "link")}, true, runnertypes.RiskLevelHigh},
		{"tar_extract_to_critical", "tar", []string{"-xf", "a.tar", "-C", "/usr/local"}, true, runnertypes.RiskLevelHigh},
		{"tar_list_not_applicable", "tar", []string{"-tf", "a.tar"}, false, runnertypes.RiskLevelLow},
		{"trailing_slash_deref", "rm", []string{etcLink + "/"}, true, runnertypes.RiskLevelHigh},
		{"chmod_setuid_grant", "chmod", []string{"u+s", filepath.Join(wd, "x")}, true, runnertypes.RiskLevelHigh},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(in, tc.cmd, tc.args...)
			assert.Equal(t, tc.wantApply, got.Applies, "Applies")
			if tc.wantApply {
				assert.Equal(t, tc.wantLevel, got.Level, "Level")
			}
		})
	}
}

// --- multiple operands max (sensitive source dominates a safe dest) ---

func TestMultipleOperandsMax(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	got := classify(in, "cp", "/etc/shadow", filepath.Join(wd, "x"))
	assert.Equal(t, runnertypes.RiskLevelMedium, got.Level,
		"safe-zone dest (Low) is dominated by the sensitive source (Medium)")
	assert.Contains(t, got.ReasonCodes, risktypes.ReasonSensitiveSourceCopy)
}

// --- permission/ownership/attribute grant floor ---

func TestFloor_PermissionGrant(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())
	safe := filepath.Join(wd, "x")

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "u+s", safe).Level, "setuid grant")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "0777", safe).Level, "world-writable")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "04755", safe).Level, "leading-zero octal setuid")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "02755", safe).Level, "leading-zero octal setgid")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chown", "root", "/usr/bin/x").Level, "trust-critical ownership change")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chattr", "-i", safe).Level, "immutable attribute change")

	// A non-granting chmod on a safe-zone file stays Low.
	assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "chmod", "0644", safe).Level, "plain mode in safe-zone")
	assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "chmod", "0755", safe).Level, "leading-zero non-granting mode in safe-zone")
}

// --- install permission flags ---

func TestFloor_InstallPermission(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())
	dst := filepath.Join(wd, "x")
	src := filepath.Join(wd, "src")
	require.NoError(t, os.WriteFile(src, nil, 0o644))

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "install", "-m", "4755", src, dst).Level, "setuid mode")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "install", "-o", "root", src, dst).Level, "owner change")
	assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "install", src, dst).Level, "plain install in safe-zone")
}

// --- dd device IO (judged by device kind, via injected lstat) ---

func TestFloor_DeviceIO(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	lstat := func(p string) (fs.FileInfo, error) {
		switch filepath.Clean(p) {
		case "/dev/sda":
			return fakeFileInfo{"sda", fs.ModeDevice | 0o660}, nil
		case "/dev/null":
			return fakeFileInfo{"null", fs.ModeDevice | fs.ModeCharDevice | 0o666}, nil
		default:
			return fakeFileInfo{filepath.Base(p), fs.ModeDir | 0o755}, nil
		}
	}
	newR := func() *operandResolver {
		return newOperandResolver(lstat, func(string) (string, error) { return "", errors.New("no symlink") })
	}

	highBlock := classifyDestinationZone(in, cmdNameSet("dd"), "dd", []string{"of=/dev/sda"}, newR())
	assert.Equal(t, runnertypes.RiskLevelHigh, highBlock.Level, "block device of= is High")
	assert.Contains(t, highBlock.ReasonCodes, risktypes.ReasonDeviceIO)

	harmless := classifyDestinationZone(in, cmdNameSet("dd"), "dd", []string{"of=/dev/null"}, newR())
	assert.Equal(t, runnertypes.RiskLevelLow, harmless.Level, "harmless sink stays Low despite /dev being critical")

	readDev := classifyDestinationZone(in, cmdNameSet("dd"), "dd", []string{"if=/dev/sda", "of=/dev/null"}, newR())
	assert.Equal(t, runnertypes.RiskLevelHigh, readDev.Level, "reading a raw device is High")
}

// --- recursion outside the safe-zone ---

func TestFloor_RecursiveOutside(t *testing.T) {
	wd := zoningWorkdir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(wd, "build"), 0o700))

	inForeign := zoningInput(wd, foreignIdent())
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(inForeign, "rm", "-rf", "/srv/app").Level,
		"recursion over an ordinary path is High")
	assert.Equal(t, runnertypes.RiskLevelLow, classify(inForeign, "rm", "-rf", filepath.Join(wd, "build")).Level,
		"recursion confined to a Trusted safe-zone stays Low")
}

// --- cp/mv/ln operand-specific rules ---

func TestOperandSpecific_CopyMoveLink(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	// Sensitive source copy -> Medium read floor.
	assert.Equal(t, runnertypes.RiskLevelMedium, classify(in, "cp", "/etc/shadow", filepath.Join(wd, "x")).Level)

	// cp -a of a setuid source -> privileged-metadata copy is High.
	suid := filepath.Join(wd, "suidsrc")
	require.NoError(t, os.WriteFile(suid, nil, 0o755))
	require.NoError(t, os.Chmod(suid, os.ModeSetuid|0o755))
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "cp", "-a", suid, filepath.Join(wd, "dst")).Level,
		"cp -a of a setuid source is High")

	// mv source is itself a write: a trust-critical move source is High.
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "mv", "/usr/bin/x", filepath.Join(wd, "dst")).Level,
		"mv from a trust-critical source is High")

	// ln to a trust-critical target is High.
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "ln", "-s", "/etc/passwd", filepath.Join(wd, "link")).Level)
}

// --- mount/umount ---

func TestOperandSpecific_Mount(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "mount", "/dev/sdb1", "/usr/local/mnt").Level,
		"mount onto a trust-critical mountpoint is High")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "umount", "-a").Level, "umount -a is unconditional High")
}

// --- tee/sponge (all FILE operands, max) ---

func TestOperandSpecific_Tee(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "tee", filepath.Join(wd, "a"), "/usr/bin/b").Level,
		"a trust-critical FILE in the tee list dominates")
	assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "tee", filepath.Join(wd, "a"), filepath.Join(wd, "b")).Level,
		"all-safe-zone tee stays Low")
}

// --- find destructive actions ---

func TestOperandSpecific_Find(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "find", "/usr", "-delete").Level,
		"find -delete from a trust-critical root is High")
	assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "find", wd, "-delete").Level,
		"find -delete confined to a Trusted safe-zone is Low")

	readOnly := classify(in, "find", "/usr", "-name", "x")
	assert.False(t, readOnly.Applies, "a read-only find does not apply (no destructive action)")

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "find", wd, "-fprintf", "/usr/bin/out", "%p").Level,
		"find -fprintf to a trust-critical destination is High")
}

// --- tar extraction is recognized (so the legacy-High downgrade path is live) ---

func TestTarExtractRecognized(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	safe := classify(in, "tar", "-xf", "a.tar", "-C", wd)
	assert.True(t, safe.Recognized, "a parseable tar extract must be fully recognized")
	assert.Equal(t, runnertypes.RiskLevelLow, safe.Level, "extracting into a Trusted safe-zone is Low")

	bundled := classify(in, "tar", "xzf", "a.tar", "-C", wd)
	assert.True(t, bundled.Recognized, "a dash-less bundled mode (xzf) is recognized")
	assert.Equal(t, runnertypes.RiskLevelLow, bundled.Level)

	crit := classify(in, "tar", "--extract", "--file", "a.tar", "-C", "/usr/local")
	assert.True(t, crit.Recognized)
	assert.Equal(t, runnertypes.RiskLevelHigh, crit.Level, "extracting into /usr is trust-critical High")
}

// --- mknod creates the named node (only NAME is a path operand) ---

func TestOperandSpecific_Mknod(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "mknod", "/dev/foo", "c", "1", "3").Level,
		"creating a node under /dev is trust-critical High")
	assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "mknod", filepath.Join(wd, "node"), "c", "1", "3").Level,
		"a node in a Trusted safe-zone is Low; TYPE/MAJOR/MINOR are not path operands")
	assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "mknod", "-m", "4755", filepath.Join(wd, "node"), "c", "1", "3").Level,
		"mknod -m setuid grants permission -> High")
}

// --- review-round extractor/recognition hardening ---

func TestExtractorHardening(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	t.Run("attached_short_value_flag_not_dropped", func(t *testing.T) {
		// tar -C/usr (attached value) must extract to /usr, not default to cwd.
		got := classify(in, "tar", "-x", "-f", "a.tar", "-C/usr/local")
		assert.True(t, got.Recognized)
		assert.Equal(t, runnertypes.RiskLevelHigh, got.Level, "attached -C value must not be dropped")
	})

	t.Run("install_directory_mode_all_write", func(t *testing.T) {
		// install -d treats every positional as a directory to create.
		got := classify(in, "install", "-d", filepath.Join(wd, "a"), "/usr/local/b")
		assert.Equal(t, runnertypes.RiskLevelHigh, got.Level, "a trust-critical dir in -d mode is High")
		require.Len(t, got.Operands, 2)
		for _, oz := range got.Operands {
			assert.Equal(t, risktypes.OperandRoleWrite, oz.Role)
		}
	})

	t.Run("chown_from_keeps_owner_spec", func(t *testing.T) {
		// `chown --from=alice bob /usr/bin/x`: bob is the owner spec, /usr/bin/x the
		// target. The owner spec must not be treated as a target.
		got := classify(in, "chown", "--from=alice", "bob", "/usr/bin/x")
		require.Len(t, got.Operands, 1)
		assert.Equal(t, "/usr/bin/x", got.Operands[0].Raw)
		assert.Equal(t, runnertypes.RiskLevelHigh, got.Level)
	})

	t.Run("find_fprintf_missing_arg_fails_closed", func(t *testing.T) {
		got := classify(in, "find", wd, "-fprintf")
		assert.False(t, got.Recognized, "missing -fprintf output file is unparsed")
		assert.Equal(t, runnertypes.RiskLevelHigh, got.Level, "unrecognized form fails closed to High")
	})

	t.Run("tee_no_file_not_applicable", func(t *testing.T) {
		got := classify(in, "tee")
		assert.False(t, got.Applies, "tee with no FILE writes only to stdout")
	})

	t.Run("ln_single_arg_records_implicit_link", func(t *testing.T) {
		// ln /usr/bin/ls -> implicit link "ls" in the work dir; both operands recorded.
		got := classify(in, "ln", "/usr/bin/ls")
		require.Len(t, got.Operands, 2)
		roles := map[risktypes.OperandRole]bool{}
		for _, oz := range got.Operands {
			roles[oz.Role] = true
		}
		assert.True(t, roles[risktypes.OperandRoleRead] && roles[risktypes.OperandRoleWrite],
			"single-arg ln records both the target (read) and the implicit link (write)")
	})

	t.Run("ln_symbolic_relative_target_resolves", func(t *testing.T) {
		// A relative symlink target resolves against the (relative) link parent,
		// anchored at the work dir; it must not become unexpectedly unresolved.
		require.NoError(t, os.MkdirAll(filepath.Join(wd, "sub"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wd, "sub", "tgt"), nil, 0o644))
		got := classify(in, "ln", "-s", "tgt", "sub/link")
		assert.True(t, got.Recognized, "a relative ln base must be anchored, not left unresolved")
		assert.Equal(t, runnertypes.RiskLevelLow, got.Level)
	})

	t.Run("unresolved_operand_sets_recognized_false", func(t *testing.T) {
		res := classifyDestinationZone(in, cmdNameSet("rm"), "rm", []string{"/x"}, erroringResolver())
		assert.False(t, res.Recognized, "an unresolved operand must defeat full recognition")
	})
}

// --- review-round 3 hardening ---

func TestExtractorHardening2(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())
	safe := filepath.Join(wd, "x")

	t.Run("chmod_symbolic_setuid_combined", func(t *testing.T) {
		// setuid 's' combined with other perms must still be detected (fail-open).
		assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "u+xs", safe).Level)
		assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "u=rws", safe).Level)
		assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "chmod", "g+rs", safe).Level)
		// removal of setuid is not a grant.
		assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "chmod", "u-s", safe).Level)
		assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "chmod", "u+rx", safe).Level)
	})

	t.Run("tar_positional_not_misread_as_mode", func(t *testing.T) {
		// "a.tar" contains 'c'/'t'; with -x before it should still be extract mode.
		got := classify(in, "tar", "-x", "-f", "a.tar", "-C", "/usr/local")
		assert.True(t, got.Recognized)
		assert.Equal(t, runnertypes.RiskLevelHigh, got.Level, "positional a.tar must not be read as mode")
	})

	t.Run("mkdir_mode_grant", func(t *testing.T) {
		assert.Equal(t, runnertypes.RiskLevelHigh, classify(in, "mkdir", "-m", "0777", safe).Level,
			"mkdir -m 0777 is a world-writable grant even in a safe-zone")
		assert.Equal(t, runnertypes.RiskLevelLow, classify(in, "mkdir", "-m", "0755", safe).Level)
	})

	t.Run("copy_target_dir_no_source_fails_closed", func(t *testing.T) {
		got := classify(in, "cp", "-t", wd)
		assert.False(t, got.Recognized, "cp -t with no source files is incomplete")
	})

	t.Run("install_target_dir_no_source_fails_closed", func(t *testing.T) {
		got := classify(in, "install", "-t", wd)
		assert.False(t, got.Recognized, "install -t with no source files is incomplete")
	})

	t.Run("dd_no_operand_not_applicable", func(t *testing.T) {
		got := classify(in, "dd", "bs=4k", "count=1")
		assert.False(t, got.Applies, "dd without if=/of= is stdin/stdout only")
	})

	t.Run("chattr_value_flag_missing_value_fails_closed", func(t *testing.T) {
		got := classify(in, "chattr", "+i", safe, "-v")
		assert.False(t, got.Recognized, "a trailing value flag with no value fails closed")
	})

	t.Run("ln_multi_target_dir_base", func(t *testing.T) {
		// ln -s a b destdir: relative targets resolve against destdir.
		require.NoError(t, os.MkdirAll(filepath.Join(wd, "dst"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(wd, "dst", "a"), nil, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(wd, "dst", "b"), nil, 0o644))
		got := classify(in, "ln", "-s", "a", "b", filepath.Join(wd, "dst"))
		assert.True(t, got.Recognized)
		assert.Equal(t, runnertypes.RiskLevelLow, got.Level)
	})
}

// --- review-round 4 hardening ---

func TestExtractorHardening3(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	t.Run("sed_script_via_e_flag", func(t *testing.T) {
		// With -e, every positional is an edited file (no inline-script positional).
		got := classify(in, "sed", "-i", "-e", "s/a/b/", "/usr/bin/x")
		assert.True(t, got.Recognized, "sed -i -e ... file must recognize the file")
		assert.Equal(t, runnertypes.RiskLevelHigh, got.Level)
	})

	t.Run("ln_long_target_directory", func(t *testing.T) {
		// --target-directory must be honored like -t.
		got := classify(in, "ln", "-s", "--target-directory", "/usr/local/bin", "/etc/passwd")
		require.NotEmpty(t, got.Operands)
		assert.Equal(t, runnertypes.RiskLevelHigh, got.Level, "link dir /usr/local/bin is trust-critical")
	})

	t.Run("tar_one_top_level_optional_arg", func(t *testing.T) {
		// Bare --one-top-level before the mode must not consume -xf; recognized.
		got := classify(in, "tar", "--one-top-level", "-xf", "a.tar")
		assert.True(t, got.Recognized, "--one-top-level is optional-arg; must not eat -xf")
		// --one-top-level=DIR uses DIR as the extraction destination.
		crit := classify(in, "tar", "--one-top-level=/usr/local", "-xf", "a.tar")
		assert.True(t, crit.Recognized)
		assert.Equal(t, runnertypes.RiskLevelHigh, crit.Level)
	})
}

func TestACLGrantsWrite_DefaultEntry(t *testing.T) {
	assert.True(t, aclGrantsWrite("default:g:staff:rwx"), "default group-write ACL is a grant")
	assert.True(t, aclGrantsWrite("d:o::rwx"), "default other-write ACL is a grant")
	assert.True(t, aclGrantsWrite("g:staff:rw"), "plain group-write ACL is a grant")
	assert.False(t, aclGrantsWrite("default:g:staff:r-x"), "default group without write is not a grant")
	assert.False(t, aclGrantsWrite("u:alice:rwx"), "a user ACL is not a group/other grant")
}

// --- data-transfer write destination (axis-2 contribution) ---

func TestDataTransferWrite(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	// curl/wget local write destination is zone-classified (egress Medium is added
	// at the evaluator layer by the network profile, not here).
	assert.Equal(t, runnertypes.RiskLevelLow,
		classify(in, "curl", "http://x/y", "-o", filepath.Join(wd, "f")).Level)
	assert.Equal(t, runnertypes.RiskLevelHigh,
		classify(in, "curl", "-o", "/usr/bin/x", "http://x/y").Level)
	assert.Equal(t, runnertypes.RiskLevelHigh,
		classify(in, "wget", "-O", "/etc/cron.d/x", "http://x/y").Level)

	// rsync to a remote daemon bare module: remote egress Medium. The remote
	// destination contributes no local WRITE operand; the local source is recorded
	// as a read operand (for sensitive-source detection and audit).
	mod := classify(in, "rsync", filepath.Join(wd, "src"), "host::module")
	assert.Equal(t, runnertypes.RiskLevelMedium, mod.Level)
	assert.Contains(t, mod.ReasonCodes, risktypes.ReasonNetworkArgument)
	assert.False(t, hasWriteOperand(mod.Operands), "a remote destination has no local write operand")

	// rsync to a remote host:path is likewise remote egress.
	assert.Equal(t, runnertypes.RiskLevelMedium,
		classify(in, "rsync", filepath.Join(wd, "src"), "host:/remote/path").Level)

	// The relative remote forms (host:file, host:, user@host:file) and bracketed
	// IPv6 forms are also egress (rsync's positional rule: a colon before the first
	// slash means remote; IPv6 hosts are bracketed).
	for _, dest := range []string{
		"host:file", "host:", "user@host:file",
		"[::1]:file", "[2001:db8::1]:/path", "user@[::1]:file",
	} {
		r := classify(in, "rsync", filepath.Join(wd, "src"), dest)
		assert.Equal(t, runnertypes.RiskLevelMedium, r.Level, "rsync to %q is remote egress", dest)
		assert.False(t, hasWriteOperand(r.Operands), "remote dest %q has no local write operand", dest)
	}

	// scp -T is boolean (disable strict filename checking); it must not consume the
	// following SRC, so a normal local-dest scp is still recognized and zoned.
	scpT := classify(in, "scp", "-T", filepath.Join(wd, "src"), "/usr/bin/x")
	assert.True(t, scpT.Recognized, "scp -T is boolean and must not shift operands")
	assert.Equal(t, runnertypes.RiskLevelHigh, scpT.Level, "scp into a trust-critical dest is High")

	// A sensitive local source uploaded to a remote destination is Medium and the
	// source is recorded (the local-source extraction closes the audit/zoning gap).
	up := classify(in, "rsync", "/etc/shadow", "host::module")
	assert.Equal(t, runnertypes.RiskLevelMedium, up.Level)
	assert.Contains(t, up.ReasonCodes, risktypes.ReasonSensitiveSourceCopy)

	// A sensitive local source copied locally into a safe-zone is Medium (parity
	// with cp), not Low: the source must be extracted and floored.
	assert.Equal(t, runnertypes.RiskLevelMedium,
		classify(in, "rsync", "/etc/shadow", filepath.Join(wd, "dst")).Level,
		"rsync of a sensitive source into a safe-zone is Medium, like cp")

	// A curl upload of a sensitive local file is flagged as a sensitive source.
	upload := classify(in, "curl", "-T", "/etc/shadow", "http://host/up")
	assert.Contains(t, upload.ReasonCodes, risktypes.ReasonSensitiveSourceCopy,
		"curl -T of a sensitive file is a sensitive-source upload")

	// Purely local rsync into a safe-zone is not over-classified.
	assert.Equal(t, runnertypes.RiskLevelLow,
		classify(in, "rsync", filepath.Join(wd, "a"), filepath.Join(wd, "b")).Level)

	// Local rsync into a trust-critical destination is High.
	assert.Equal(t, runnertypes.RiskLevelHigh,
		classify(in, "rsync", filepath.Join(wd, "a"), "/usr/bin/x").Level)
}

// --- carrier empty vs applied-but-unresolved ---

func TestOperandZones_EmptyVsUnresolved(t *testing.T) {
	wd := zoningWorkdir(t)
	in := zoningInput(wd, foreignIdent())

	// A non-file-operation command does not apply: the carrier is empty.
	notFileOp := classify(in, "echo", "hello")
	assert.False(t, notFileOp.Applies)
	assert.Empty(t, notFileOp.Operands, "axis 2 did not apply -> empty carrier")

	// An applied-but-unresolvable operand stays as a ZoneUnresolved element.
	unres := classifyDestinationZone(in, cmdNameSet("rm"), "rm", []string{"/x"}, erroringResolver())
	assert.True(t, unres.Applies)
	require.Len(t, unres.Operands, 1)
	assert.Equal(t, risktypes.ZoneUnresolved, unres.Operands[0].Zone)

	// Per-operand audit fields are populated for a resolved operand.
	resolved := classify(in, "cp", "/etc/shadow", filepath.Join(wd, "x"))
	require.Len(t, resolved.Operands, 2)
	var dst, src risktypes.OperandZone
	for _, oz := range resolved.Operands {
		if oz.Role == risktypes.OperandRoleWrite {
			dst = oz
		} else {
			src = oz
		}
	}
	assert.Equal(t, filepath.Join(wd, "x"), dst.Resolved)
	assert.Equal(t, risktypes.ZoneSafeZone, dst.Zone)
	assert.True(t, dst.Trusted)
	assert.Equal(t, "/etc/shadow", src.Resolved)
	assert.Equal(t, risktypes.ZoneTrustCritical, src.Zone)
}

// TestResolutionCeiling: the operand-count ceiling fails closed -- a command with
// more operands than MaxOperands is forced to High and not Recognized rather than
// walking the filesystem unboundedly (DoS guard on the ExecuteCommand hot path).
// The companion symlink-hop ceiling is covered by TestResolveOperandPath_DepthExceeded
// and TestResolveOperandPath_Cycle.
func TestResolutionCeiling(t *testing.T) {
	in := zoningInput(zoningWorkdir(t), foreignIdent())
	in.MaxOperands = 2

	over := classify(in, "rm", "/srv/a", "/srv/b", "/srv/c")
	assert.Equal(t, runnertypes.RiskLevelHigh, over.Level, "exceeding the operand ceiling fails closed to High")
	assert.False(t, over.Recognized, "an over-ceiling command is not fully recognized")
	assert.Contains(t, over.ReasonCodes, risktypes.ReasonUnresolvedDestination)
	assert.Empty(t, over.Operands, "the ceiling short-circuits before per-operand resolution")

	// At the ceiling the same command classifies normally (the ceiling does not
	// itself force High): two ordinary deletes are Medium and Recognized.
	within := classify(in, "rm", "/srv/a", "/srv/b")
	assert.Equal(t, runnertypes.RiskLevelMedium, within.Level, "within the ceiling, ordinary deletes are Medium")
	assert.True(t, within.Recognized)
}
