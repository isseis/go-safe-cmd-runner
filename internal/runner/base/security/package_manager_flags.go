package security

import "strings"

// flagRule describes how one flag-style package manager selects a modifying
// operation from its option flags. All character matching is case-sensitive.
type flagRule struct {
	// modifyingShortChars: for a short-option token (-X, not --X), if its FIRST
	// character (immediately after the single dash) is one of these, the token
	// marks a modifying operation (pacman "SRU", dpkg "irP", rpm "iUFe"). The mode
	// selector is always the first short-option character in these tools, so
	// first-character matching avoids false positives from concatenated argument
	// values (e.g. rpm -E%{_libdir}) or single-dash long forms (e.g. dpkg -list).
	modifyingShortChars string

	// modifyingLongForms: exact-match long options that are modifying
	// (e.g. "--install", "--purge", "--reinstall"). Matched whole-token.
	modifyingLongForms map[string]struct{}

	// excludeShortChars / excludeLongForms: if ANY token in args carries one of
	// these query/verify markers, the command is treated as non-modifying
	// regardless of modifying flags (rpm: short "qV", long "--query"/"--verify";
	// empty for pacman/dpkg). This encodes the least-privilege query exception.
	excludeShortChars string
	excludeLongForms  map[string]struct{}
}

// flagStyleManagers maps a manager basename to its flag-style detection rule.
// Adding a new flag-style package manager means adding one entry here; the
// detection logic in isSystemModificationByNames stays unchanged.
var flagStyleManagers = map[string]flagRule{
	"pacman": {
		// pacman selects its operation via -S (sync/install), -R (remove), or
		// -U (upgrade), possibly combined (-Syu, -Rns), or via the long forms
		// --sync/--remove/--upgrade. No query/verify exclusion.
		modifyingShortChars: "SRU",
		modifyingLongForms: map[string]struct{}{
			"--sync":    {},
			"--remove":  {},
			"--upgrade": {},
		},
	},
	"dpkg": {
		// dpkg selects its action with a single short option whose first
		// character is -i (install), -r (remove), or -P (purge), or with the long
		// forms below. Query options (-l/-L/-s/-S/-p/-I/-c, --info/--list/...)
		// start with a different first character, so no exclusion is needed.
		modifyingShortChars: "irP",
		modifyingLongForms: map[string]struct{}{
			"--install":   {},
			"--remove":    {},
			"--purge":     {},
			"--unpack":    {},
			"--configure": {},
		},
	},
	"rpm": {
		// rpm selects its mode with a leading short option -i (install),
		// -U (upgrade), -F (freshen), or -e (erase), or with a long form. Query
		// and verify modes (-q/-V, --query/--verify) are read-only; if any token
		// carries one, the command is treated as non-modifying (least privilege)
		// even when a modifying flag is also present.
		modifyingShortChars: "iUFe",
		modifyingLongForms: map[string]struct{}{
			"--install":   {},
			"--upgrade":   {},
			"--freshen":   {},
			"--erase":     {},
			"--reinstall": {},
			"--import":    {},
			"--initdb":    {},
			"--rebuilddb": {},
			// --setperms/--setugids rewrite installed-file permissions and
			// ownership, so they modify system state even without (un)installing.
			"--setperms": {},
			"--setugids": {},
		},
		excludeShortChars: "qV",
		excludeLongForms: map[string]struct{}{
			"--query":  {},
			"--verify": {},
		},
	},
}

// isFlagStyleModification reports whether args invoke a modifying operation under
// the given flag-style rule. A query/verify exclusion token (if the rule defines
// any) takes precedence over modifying flags, returning false. Degenerate tokens
// (a lone "-", a lone "--", or an empty string) match neither modifying nor
// exclude flags; the length checks below guard against out-of-range indexing.
func isFlagStyleModification(rule flagRule, args []string) bool {
	if rule.excludeShortChars != "" || len(rule.excludeLongForms) > 0 {
		for _, arg := range args {
			if matchesShortFlag(arg, rule.excludeShortChars) {
				return false
			}
			if _, ok := rule.excludeLongForms[arg]; ok {
				return false
			}
		}
	}

	for _, arg := range args {
		if _, ok := rule.modifyingLongForms[arg]; ok {
			return true
		}
		if matchesShortFlag(arg, rule.modifyingShortChars) {
			return true
		}
	}

	return false
}

// matchesShortFlag reports whether arg is a short-option token (a single leading
// dash, not "--") whose first character is in chars. It returns false for long
// options ("--..."), degenerate tokens (lone "-", empty string), and when chars
// is empty, so callers need not pre-validate the token shape.
func matchesShortFlag(arg, chars string) bool {
	if chars == "" {
		return false
	}
	if len(arg) <= 1 || arg[0] != '-' || arg[1] == '-' {
		return false
	}
	return strings.IndexByte(chars, arg[1]) >= 0
}
