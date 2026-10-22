package parser

import (
	"regexp"
	"strings"

	"ecrmap/pkg/model"
)

// Regex for standard AWS ECR private registries:
// e.g. 123456789012.dkr.ecr.us-east-1.amazonaws.com/repo-name
var ecrPrivateRegex = regexp.MustCompile(`^(\d{12})\.dkr\.ecr(-fips)?\.([a-z0-9-]+)\.amazonaws\.com(?:\.cn)?/(.+)$`)

// Regex for AWS ECR public:
// e.g. public.ecr.aws/alias/repo-name
var ecrPublicRegex = regexp.MustCompile(`^public\.ecr\.aws/([^/]+)/(.+)$`)

// ParseImageURI parses a full container image URI into an ImageRef.
func ParseImageURI(rawURI string) model.ImageRef {
	ref := model.ImageRef{
		RawURI: rawURI,
	}

	remaining := rawURI

	// 1. Extract digest if present (e.g. @sha256:...)
	if atIdx := strings.Index(remaining, "@"); atIdx != -1 {
		ref.Digest = remaining[atIdx+1:]
		remaining = remaining[:atIdx]
	}

	// 2. Extract tag if present (after the last colon, but ensure it's not part of the host:port)
	// Example: host:port/repo:tag -> find colon after slash
	slashIdx := strings.LastIndex(remaining, "/")
	var tagPart string
	if slashIdx != -1 {
		colonIdx := strings.LastIndex(remaining[slashIdx:], ":")
		if colonIdx != -1 {
			tagPart = remaining[slashIdx+colonIdx+1:]
			remaining = remaining[:slashIdx+colonIdx]
		}
	} else {
		// e.g. "nginx:latest" or "nginx"
		colonIdx := strings.LastIndex(remaining, ":")
		if colonIdx != -1 {
			tagPart = remaining[colonIdx+1:]
			remaining = remaining[:colonIdx]
		}
	}
	ref.Tag = tagPart

	// 3. Now remaining is [registry/]repository
	if matches := ecrPrivateRegex.FindStringSubmatch(rawURI[:strings.Index(rawURI, remaining)+len(remaining)]); len(matches) > 0 {
		ref.IsECR = true
		ref.AccountID = matches[1]
		ref.Region = matches[3]
		ref.Registry = matches[1] + ".dkr.ecr." + matches[3] + ".amazonaws.com"
		ref.Repository = matches[4]
		return ref
	}

	if matches := ecrPublicRegex.FindStringSubmatch(rawURI[:strings.Index(rawURI, remaining)+len(remaining)]); len(matches) > 0 {
		ref.IsECR = true
		ref.Registry = "public.ecr.aws/" + matches[1]
		ref.Repository = matches[2]
		return ref
	}

	// Generic image (Docker Hub, Quay, etc.)
	parts := strings.Split(remaining, "/")
	if len(parts) == 1 {
		ref.Registry = "docker.io"
		ref.Repository = parts[0]
	} else if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") || parts[0] == "localhost" {
		ref.Registry = parts[0]
		ref.Repository = strings.Join(parts[1:], "/")
	} else {
		ref.Registry = "docker.io"
		ref.Repository = remaining
	}

	return ref
}

// Matches returns true if an ImageRef matches a user search term.
// The search term can be:
// - exact tag: "v1.2.3"
// - repo:tag: "payment-api:v1.2.3"
// - repo only: "payment-api"
// - digest: "sha256:abcd..."
// - full raw URI
func Matches(ref model.ImageRef, query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return false
	}

	// 1. Direct match with raw URI
	if strings.EqualFold(ref.RawURI, q) {
		return true
	}

	// 2. Direct match with digest or prefix of digest
	if ref.Digest != "" {
		if strings.EqualFold(ref.Digest, q) || strings.HasPrefix(strings.ToLower(ref.Digest), strings.ToLower(q)) {
			return true
		}
		// Match without "sha256:" prefix
		digestHash := strings.TrimPrefix(ref.Digest, "sha256:")
		qHash := strings.TrimPrefix(q, "sha256:")
		if strings.HasPrefix(strings.ToLower(digestHash), strings.ToLower(qHash)) {
			return true
		}
	}

	// 3. Match repo:tag
	repoTag := ref.Repository
	if ref.Tag != "" {
		repoTag = ref.Repository + ":" + ref.Tag
		if strings.EqualFold(repoTag, q) {
			return true
		}
	}

	// 4. Match repository only (or end of repository if nested)
	if strings.EqualFold(ref.Repository, q) {
		return true
	}
	repoParts := strings.Split(ref.Repository, "/")
	if len(repoParts) > 1 && strings.EqualFold(repoParts[len(repoParts)-1], q) {
		return true
	}

	// 5. If query contains a colon, check if query matches short repo:tag
	if strings.Contains(q, ":") {
		parts := strings.SplitN(q, ":", 2)
		targetRepo := parts[0]
		targetTag := parts[1]
		if (strings.EqualFold(ref.Repository, targetRepo) || strings.EqualFold(repoParts[len(repoParts)-1], targetRepo)) &&
			strings.EqualFold(ref.Tag, targetTag) {
			return true
		}
	}

	return false
}
