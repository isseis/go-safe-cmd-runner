package elfanalyzer

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
// This is used by StandardELFAnalyzer for symbol matching.
func GetNetworkSymbols() map[string]SymbolCategory {
	// Return a copy to prevent external modification
	result := make(map[string]SymbolCategory, len(networkSymbolRegistry))
	for k, v := range networkSymbolRegistry {
		result[k] = v
	}
	return result
}

// IsNetworkSymbol checks if the given symbol name is a known network symbol.
// Returns the category if found, empty string otherwise.
func IsNetworkSymbol(name string) (SymbolCategory, bool) {
	cat, found := networkSymbolRegistry[name]
	return cat, found
}

// SymbolCount returns the number of registered network symbols.
// Useful for testing and documentation.
func SymbolCount() int {
	return len(networkSymbolRegistry)
}
