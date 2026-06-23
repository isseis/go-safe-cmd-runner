package security

import (
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// LocationKind classifies a file-operation command for operand extraction. Each
// kind maps to an extraction rule in the single spec table. Unknown or ambiguous
// forms fall through to ZoneUnresolved (fail-closed).
type LocationKind int

const (
	// KindNone marks a command that is not a file operation (axis 2 does not apply).
	KindNone LocationKind = iota
	// KindCopyMove is cp/mv: destination plus source(s).
	KindCopyMove
	// KindRemove is rm/rmdir/unlink/shred: all operands.
	KindRemove
	// KindLink is ln: link target plus link name.
	KindLink
	// KindInPlaceEdit is truncate / sed -i: the edited FILE.
	KindInPlaceEdit
	// KindWriteFile is touch/mkdir/install/tee/sponge.
	KindWriteFile
	// KindArchiveExtract is tar -x / unzip: the extraction directory.
	KindArchiveExtract
	// KindDeviceIO is dd: if=/of= by device kind.
	KindDeviceIO
	// KindMount is mount/umount: mountpoint plus source.
	KindMount
	// KindPermission is chmod/chown/chgrp/setfacl/chattr.
	KindPermission
	// KindFindDestructive is find with a destructive/writing action.
	KindFindDestructive
	// KindDataTransferWrite is curl -o/-O, wget, scp/sftp dest, rsync DEST.
	// Its extraction and name-floor composition are added in a later phase.
	KindDataTransferWrite
)

// ZoningInput is the precomputed, pure input to the zoning judgment. Every
// environment-dependent value is injected here so the judgment is deterministic
// and free of live identity.
type ZoningInput struct {
	EffectiveWorkDir           string               // safe-zone origin (RuntimeCommand.EffectiveWorkDir)
	DedicatedTempDir           string               // configured dedicated temp (safe-zone origin)
	SystemCriticalPaths        []string             // trust-critical roots
	TrustedDirectories         []string             // trusted-directory allowlist
	RunAsIdent                 risktypes.RunAsIdent // injected identity for the Trusted predicate
	OutputCriticalPathPatterns []string             // sensitive-file substrings (sensitive-source-copy floor)
	MaxOperands                int                  // resolution-cost ceiling on operand count
	MaxSymlinkHops             int                  // per-operand symlink-hop ceiling
}

// LocationResult is the axis-2 verdict for one command.
type LocationResult struct {
	Applies     bool                    // true when the command is a file-operation command
	Recognized  bool                    // full recognition: all operands resolved AND all argv parsed
	Level       runnertypes.RiskLevel   // max across operands and operation-specific floors
	Operands    []risktypes.OperandZone // per-operand audit records (carrier)
	ReasonCodes []risktypes.ReasonCode  // zone-derived and floor reason codes
}

// ClassifyDestinationZone extracts the acting operands of a file-operation
// command, resolves each to an absolute symlink-followed path, classifies its
// trust zone, applies operation-specific floors, and folds the per-operand max.
// It reads no live identity and performs no writes (read-only lstat/readlink).
func ClassifyDestinationZone(input ZoningInput, names map[string]struct{}, cmdPath string, args []string) LocationResult {
	return classifyDestinationZone(input, names, cmdPath, args, newOperandResolver(os.Lstat, os.Readlink))
}

// classifyDestinationZone is the resolver-injectable core, so tests can substitute
// the read-only lstat/readlink primitives (e.g. to present a device node).
func classifyDestinationZone(input ZoningInput, names map[string]struct{}, cmdPath string, args []string, r *operandResolver) LocationResult {
	spec, ok := lookupSpec(names, cmdPath)
	if !ok {
		// Not a file-operation command: axis 2 does not apply. An empty Operands
		// slice is the contract for "did not apply" (distinct from an applied but
		// unresolved operand).
		return LocationResult{Applies: false}
	}

	ext := spec.extract(args)
	if !ext.applies {
		// A known command in a non-writing form (sed without -i, tar -t, unzip -l,
		// find without a destructive action): axis 2 does not apply.
		return LocationResult{Applies: false}
	}

	res := LocationResult{Applies: true, Recognized: true, Level: runnertypes.RiskLevelLow}
	if !ext.recognized {
		res.Recognized = false
	}

	// umount -a affects every mount unconditionally.
	if ext.umountAll {
		res.Level = runnertypes.RiskLevelHigh
		res.ReasonCodes = appendReason(res.ReasonCodes, risktypes.ReasonTrustBoundaryWrite)
	}

	// A data-transfer command writing to a remote destination has no local path to
	// zone-classify; it contributes a network-egress Medium floor. For a remote
	// rsync host::module this is the only egress signal, since the global
	// network-argument check intentionally does not match a bare module.
	if ext.remoteEgress {
		if res.Level < runnertypes.RiskLevelMedium {
			res.Level = runnertypes.RiskLevelMedium
		}
		res.ReasonCodes = appendReason(res.ReasonCodes, risktypes.ReasonNetworkArgument)
	}

	// Resolution-cost ceiling: too many operands fails closed rather than walking
	// the filesystem unboundedly.
	if input.MaxOperands > 0 && len(ext.operands) > input.MaxOperands {
		res.Recognized = false
		res.Level = runnertypes.RiskLevelHigh
		res.ReasonCodes = appendReason(res.ReasonCodes, risktypes.ReasonUnresolvedDestination)
		return res
	}

	for idx, op := range ext.operands {
		oz := r.classifyOperand(idx, op, spec, input)
		res.Operands = append(res.Operands, oz)

		// Full recognition requires every operand to resolve: an unresolved operand
		// must not let a later phase treat the command as fully recognized and
		// suppress the legacy High dimensions (fail-open).
		if oz.Zone == risktypes.ZoneUnresolved {
			res.Recognized = false
		}

		if spec.kind == KindDeviceIO {
			// dd operands are judged by device KIND, not by the path's zone: a
			// harmless sink (/dev/null) stays Low even though /dev is a critical
			// path, and a raw device is High. This is the one place where the level
			// can be below the path zone, so it is computed instead of (not in
			// addition to) the zone level + floors.
			lvl, reason := r.deviceOperandLevel(oz, op, input)
			res.Level = max(res.Level, lvl)
			res.ReasonCodes = appendReason(res.ReasonCodes, reason)
			continue
		}

		res.Level = max(res.Level, zoneLevel(oz.Zone, oz.Role, oz.Trusted))
		res.ReasonCodes = appendReason(res.ReasonCodes, zoneReason(oz))

		// Operation-specific floors (applied after the zone level; a safe-zone
		// operand is not demoted to Low by them).
		floor, freason := r.operandFloor(oz, op, spec, ext, input)
		if floor > res.Level {
			res.Level = floor
		}
		if floor >= runnertypes.RiskLevelMedium && freason != "" {
			res.ReasonCodes = appendReason(res.ReasonCodes, freason)
		}
	}

	// Incomplete recognition fails closed: a form we could not fully parse must not
	// pass as Low. The legacy High suppression (a later phase) only happens on full
	// recognition, so the floor here keeps an unparsed form High on its own.
	if !res.Recognized {
		if res.Level < runnertypes.RiskLevelHigh {
			res.Level = runnertypes.RiskLevelHigh
		}
		res.ReasonCodes = appendReason(res.ReasonCodes, risktypes.ReasonUnresolvedDestination)
	}

	return res
}

// classifyOperand resolves one operand and classifies its trust zone, recording
// the per-operand audit fields.
func (r *operandResolver) classifyOperand(idx int, op rawOperand, _ commandSpec, input ZoningInput) risktypes.OperandZone {
	oz := risktypes.OperandZone{Index: idx, Raw: op.raw, Role: op.role}

	base := input.EffectiveWorkDir
	if op.base != "" {
		base = op.base
		// A per-operand base (e.g. an ln link's parent directory) may be relative;
		// anchor it at the working directory so the resolver gets an absolute base.
		if !filepath.IsAbs(base) {
			base = filepath.Join(input.EffectiveWorkDir, base)
		}
	}
	resolved, err := r.resolve(op.raw, base, input.MaxSymlinkHops)
	if err != nil {
		oz.Zone = risktypes.ZoneUnresolved
		oz.UnresolvedErr = err.Error()
		return oz
	}
	oz.Resolved = resolved

	zone, matched, trusted := r.classifyZone(resolved, input)
	oz.Zone = zone
	oz.MatchedCritical = matched
	oz.Trusted = trusted
	return oz
}

// classifyZone maps a resolved absolute path to its trust zone. trust-critical
// takes precedence over safe-zone when a configured critical path overlaps an
// origin. "/" matches only by exact equality, never as a containing prefix.
func (r *operandResolver) classifyZone(resolved string, input ZoningInput) (zone risktypes.PathTrustZone, matchedCritical string, trusted bool) {
	clean := filepath.Clean(resolved)

	for _, c := range input.SystemCriticalPaths {
		if c == "" {
			continue
		}
		// Clean the configured path so a trailing slash or "." segment does not
		// defeat the exact-"/" check or the containment comparison.
		cleanC := filepath.Clean(c)
		if cleanC == string(filepath.Separator) {
			if clean == string(filepath.Separator) {
				return risktypes.ZoneTrustCritical, c, false
			}
			continue
		}
		if clean == cleanC || common.IsPathWithinDirectory(clean, cleanC) {
			return risktypes.ZoneTrustCritical, c, false
		}
	}

	for _, origin := range []string{input.EffectiveWorkDir, input.DedicatedTempDir} {
		if origin == "" {
			continue
		}
		// A non-absolute origin cannot anchor a safe-zone: ancestor traversal would
		// terminate early and the containment check is meaningless. Skip it so the
		// operand classifies ordinary rather than a spuriously Trusted safe-zone.
		if !filepath.IsAbs(origin) {
			continue
		}
		cleanOrigin := filepath.Clean(origin)
		if clean == cleanOrigin || common.IsPathWithinDirectory(clean, cleanOrigin) {
			t := r.isTrustedOperand(clean, cleanOrigin, input.TrustedDirectories, input.RunAsIdent)
			return risktypes.ZoneSafeZone, "", t
		}
	}

	return risktypes.ZoneOrdinary, "", false
}

// zoneLevel maps a classified zone (with role and Trusted) to a risk level.
func zoneLevel(zone risktypes.PathTrustZone, role risktypes.OperandRole, trusted bool) runnertypes.RiskLevel {
	switch zone {
	case risktypes.ZoneTrustCritical:
		// A write/delete to a system-critical path is destruction (High); reading
		// one is information exposure (Medium), raised further by the
		// sensitive-source floor when the content is secret.
		if role == risktypes.OperandRoleRead {
			return runnertypes.RiskLevelMedium
		}
		return runnertypes.RiskLevelHigh
	case risktypes.ZoneOrdinary:
		return runnertypes.RiskLevelMedium
	case risktypes.ZoneSafeZone:
		if trusted {
			return runnertypes.RiskLevelLow
		}
		return runnertypes.RiskLevelMedium
	case risktypes.ZoneUnresolved:
		// Asymmetric fail-closed: a write/delete to an unknown location is worst
		// case destruction (High); a read is information exposure (Medium).
		if role == risktypes.OperandRoleRead {
			return runnertypes.RiskLevelMedium
		}
		return runnertypes.RiskLevelHigh
	default:
		return runnertypes.RiskLevelMedium
	}
}

// zoneReason returns the reason code that explains an operand's zone-derived level.
func zoneReason(oz risktypes.OperandZone) risktypes.ReasonCode {
	switch oz.Zone {
	case risktypes.ZoneTrustCritical:
		if oz.Role == risktypes.OperandRoleRead {
			// The read reason is supplied by the sensitive-source floor.
			return ""
		}
		return risktypes.ReasonTrustBoundaryWrite
	case risktypes.ZoneOrdinary:
		return risktypes.ReasonDestinationZone
	case risktypes.ZoneUnresolved:
		return risktypes.ReasonUnresolvedDestination
	default:
		return ""
	}
}

func appendReason(codes []risktypes.ReasonCode, code risktypes.ReasonCode) []risktypes.ReasonCode {
	if code == "" {
		return codes
	}
	for _, c := range codes {
		if c == code {
			return codes
		}
	}
	return append(codes, code)
}
