// Package version reads and compares the app version, MAJOR.MINOR.PATCH
// (docs/adr/0002-versioning-and-releases.md).
package version

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/acoradd/otter/src/internal/apperr"
)

// Dev is the version of builds made outside a release.
const Dev = "dev"

// CodeInvalidVersion reports a version not in the MAJOR.MINOR.PATCH form.
const CodeInvalidVersion apperr.Code = "INVALID_VERSION"

// Version is a parsed MAJOR.MINOR.PATCH.
type Version struct {
	Major, Minor, Patch int
}

// Parse reads "1.0.1". Leading zeros, signs and suffixes are refused: release
// tags must be exact.
func Parse(s string) (Version, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, apperr.New(CodeInvalidVersion)
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 || p != strconv.Itoa(v) {
			return Version{}, apperr.New(CodeInvalidVersion)
		}
		n[i] = v
	}
	return Version{n[0], n[1], n[2]}, nil
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Less reports whether v comes before other.
func (v Version) Less(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}
