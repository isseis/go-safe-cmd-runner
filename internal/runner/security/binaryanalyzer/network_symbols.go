package binaryanalyzer

import (
	"maps"
	"slices"
)

// SymbolCategory represents the category of a network-related symbol.
type SymbolCategory string

const (
	// CategorySocket represents POSIX socket API functions.
	CategorySocket SymbolCategory = "socket"

	// CategoryHTTP represents HTTP client library functions.
	CategoryHTTP SymbolCategory = "http"

	// CategoryTLS represents TLS/SSL library functions.
	CategoryTLS SymbolCategory = "tls"

	// CategoryDNS represents DNS resolution functions.
	CategoryDNS SymbolCategory = "dns"

	// CategoryDynamicLoad represents dynamic library loading functions (dlopen/dlsym/dlvsym).
	CategoryDynamicLoad SymbolCategory = "dynamic_load"

	// CategorySyscallWrapper represents libc/libSystem symbols that are not network-related.
	// These are syscall wrapper functions imported from libc (ELF) or libSystem (Mach-O).
	CategorySyscallWrapper SymbolCategory = "syscall_wrapper"
)

// networkSymbolRegistry contains the default set of network-related symbols.
// Key: symbol name, Value: category
//
// Symbol names should NOT include version suffixes (e.g., @GLIBC_2.2.5)
// as Go's debug/elf.DynamicSymbols() returns names without versioning.
var networkSymbolRegistry = map[string]SymbolCategory{
	// =========================================
	// Socket API (POSIX)
	// =========================================

	// Socket creation and connection
	"socket":  CategorySocket,
	"connect": CategorySocket,
	"bind":    CategorySocket,
	"listen":  CategorySocket,
	"accept":  CategorySocket,
	"accept4": CategorySocket, // Linux-specific

	// Data transmission
	"send":     CategorySocket,
	"sendto":   CategorySocket,
	"sendmsg":  CategorySocket,
	"recv":     CategorySocket,
	"recvfrom": CategorySocket,
	"recvmsg":  CategorySocket,
	"sendmmsg": CategorySocket, // Linux-specific
	"recvmmsg": CategorySocket, // Linux-specific

	// Socket information
	"getpeername": CategorySocket,
	"getsockname": CategorySocket,

	// Address conversion
	"inet_ntop": CategorySocket,
	"inet_pton": CategorySocket,

	// =========================================
	// DNS Resolution
	// =========================================

	"getaddrinfo":    CategoryDNS,
	"getnameinfo":    CategoryDNS,
	"gethostbyname":  CategoryDNS, // Legacy, but still widely used
	"gethostbyname2": CategoryDNS, // IPv4/IPv6 variant
	"gethostbyaddr":  CategoryDNS,
	"res_init":       CategoryDNS,
	"res_query":      CategoryDNS,
	"res_search":     CategoryDNS,

	// =========================================
	// HTTP Libraries (libcurl)
	// =========================================

	"curl_easy_init":     CategoryHTTP,
	"curl_easy_perform":  CategoryHTTP,
	"curl_easy_cleanup":  CategoryHTTP,
	"curl_multi_init":    CategoryHTTP,
	"curl_multi_perform": CategoryHTTP,
	"curl_multi_cleanup": CategoryHTTP,
	"curl_global_init":   CategoryHTTP,

	// =========================================
	// TLS/SSL Libraries (OpenSSL)
	// =========================================

	"SSL_new":          CategoryTLS,
	"SSL_connect":      CategoryTLS,
	"SSL_accept":       CategoryTLS,
	"SSL_read":         CategoryTLS,
	"SSL_write":        CategoryTLS,
	"SSL_shutdown":     CategoryTLS,
	"SSL_free":         CategoryTLS,
	"SSL_CTX_new":      CategoryTLS,
	"SSL_CTX_free":     CategoryTLS,
	"SSL_library_init": CategoryTLS, // Legacy OpenSSL 1.0
	"OPENSSL_init_ssl": CategoryTLS, // OpenSSL 1.1+

	// =========================================
	// TLS/SSL Libraries (GnuTLS)
	// =========================================

	"gnutls_init":        CategoryTLS,
	"gnutls_handshake":   CategoryTLS,
	"gnutls_record_send": CategoryTLS,
	"gnutls_record_recv": CategoryTLS,
	"gnutls_bye":         CategoryTLS,
	"gnutls_deinit":      CategoryTLS,
	"gnutls_global_init": CategoryTLS,
}

// GetNetworkSymbols returns a copy of the network symbol registry.
// This is used by binary analyzers for symbol matching.
func GetNetworkSymbols() map[string]SymbolCategory {
	// Return a copy to prevent external modification
	result := make(map[string]SymbolCategory, len(networkSymbolRegistry))
	maps.Copy(result, networkSymbolRegistry)
	return result
}

// IsNetworkSymbol checks if the given symbol name is a known network symbol.
// Returns the category if found, empty string otherwise.
func IsNetworkSymbol(name string) (SymbolCategory, bool) {
	cat, found := networkSymbolRegistry[name]
	return cat, found
}

// IsNetworkCategory returns true if the given category string represents a
// network-related symbol category. The network categories are "socket", "dns",
// "tls", and "http". "syscall_wrapper" and "dynamic_load" return false.
func IsNetworkCategory(cat string) bool {
	switch SymbolCategory(cat) {
	case CategorySocket, CategoryDNS, CategoryTLS, CategoryHTTP:
		return true
	}
	return false
}

// dynamicLoadSymbolRegistry contains symbols for dynamic library loading.
var dynamicLoadSymbolRegistry = map[string]struct{}{
	"dlopen": {},
	"dlsym":  {},
	"dlvsym": {},
}

// IsDynamicLoadSymbol returns true if the given symbol name is a dynamic library
// loading function (dlopen, dlsym, or dlvsym).
func IsDynamicLoadSymbol(name string) bool {
	_, found := dynamicLoadSymbolRegistry[name]
	return found
}

// DynamicLoadSymbolNames returns the sorted list of dynamic-load symbol names
// registered in dynamicLoadSymbolRegistry. Use this to build log messages or
// documentation so that they automatically stay in sync with the registry.
func DynamicLoadSymbolNames() []string {
	names := make([]string, 0, len(dynamicLoadSymbolRegistry))
	for name := range dynamicLoadSymbolRegistry {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
