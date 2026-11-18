#!/bin/sh
# test-commit-identity.sh — positive/negative matrix for scripts/check-commit-identity.mjs
#
# Builds throwaway git repositories with synthetic identities only. Verifies that
# the identity gate rejects unlinked automation placeholders and AI co-author
# trailers, accepts the maintainer and explicitly authorized automation, and does
# not block genuine external contributors in permissive mode.
#
# Usage: sh scripts/ops/test-commit-identity.sh
# Exit 0 iff every case passes.

set -u

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
SCRIPT="$REPO_ROOT/scripts/check-commit-identity.mjs"

PASS=0
FAIL=0

assert() { # name expected actual
	if [ "$2" -eq "$3" ]; then
		PASS=$((PASS + 1))
		printf 'PASS  %s (exit %s)\n' "$1" "$3"
	else
		FAIL=$((FAIL + 1))
		printf 'FAIL  %s: expected exit %s, got %s\n' "$1" "$2" "$3"
		[ -f "$OUT" ] && sed -n '1,12p' "$OUT"
	fi
}

newrepo() {
	TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-identity-$$")
	mkdir -p "$TMP"
	OUT="$TMP/out"
	REPO="$TMP/repo"
	git init -q "$REPO" || exit 1
	cd "$REPO" || exit 1
	git config user.name 'base'
	git config user.email 'base@example.org'
	printf 'base\n' >file.txt
	git add file.txt
	git commit -q -m 'base'
	BASE=$(git rev-parse HEAD)
}

commit_as() { # author_name author_email committer_name committer_email [trailer]
	an=$1 ae=$2 cn=$3 ce=$4 trailer=${5:-}
	set -- -m 'case commit'
	if [ -n "$trailer" ]; then set -- "$@" -m "$trailer"; fi
	GIT_AUTHOR_NAME="$an" GIT_AUTHOR_EMAIL="$ae" \
	GIT_COMMITTER_NAME="$cn" GIT_COMMITTER_EMAIL="$ce" \
	git commit -q --allow-empty "$@"
}

run_gate() { # mode [env assignment string...]
	mode=$1
	shift
	env "$@" node "$SCRIPT" --range "$BASE..HEAD" --mode "$mode" >"$OUT" 2>&1
	echo $?
}

# 1. Maintainer identity, strict -> PASS
newrepo
commit_as 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id' 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id'
assert 'maintainer author+committer, strict' 0 "$(run_gate strict)"

# 2. GitHub noreply maintainer identity, strict -> PASS
newrepo
commit_as 'lostsky1403' '114224549+lostsky1403@users.noreply.github.com' 'lostsky1403' '114224549+lostsky1403@users.noreply.github.com'
assert 'maintainer noreply identity, strict' 0 "$(run_gate strict)"

# 3. Explicitly authorized automation identity, strict -> PASS
newrepo
commit_as 'sigap-ci' 'sigap-ci@automation.sigap.example' 'sigap-ci' 'sigap-ci@automation.sigap.example'
assert 'authorized automation, strict' 0 "$(run_gate strict SIGAP_AUTHORIZED_COMMITTERS='sigap-ci@automation.sigap.example')"

# 4. Genuine external contributor, permissive -> PASS (not blocked for a different email)
newrepo
commit_as 'Jean-Claude Van Damme' 'jean@example.org' 'Jean-Claude Van Damme' 'jean@example.org'
assert 'external human, permissive' 0 "$(run_gate permissive)"

# 5. Legitimate human co-author trailer -> PASS (delegated trailer gate preserves humans)
newrepo
commit_as 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id' 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id' 'Co-authored-by: Real Person <real@example.com>'
assert 'legitimate human co-author trailer' 0 "$(run_gate strict)"

# 6. Unlinked sigap-ops AUTHOR, permissive -> FAIL
newrepo
commit_as 'sigap-ops' 'ops@local' 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id'
assert 'sigap-ops author placeholder' 1 "$(run_gate permissive)"

# 7. Unlinked sigap-ops COMMITTER, permissive -> FAIL
newrepo
commit_as 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id' 'sigap-ops' 'ops@local'
assert 'sigap-ops committer placeholder' 1 "$(run_gate permissive)"

# 8. Generic localhost automation identity -> FAIL
newrepo
commit_as 'root' 'root@localhost' 'root' 'root@localhost'
assert 'root@localhost placeholder' 1 "$(run_gate permissive)"

# 9. Unknown external email under strict (trusted branch) -> FAIL
newrepo
commit_as 'External Person' 'external@example.org' 'External Person' 'external@example.org'
assert 'unknown email under strict' 1 "$(run_gate strict)"

# 10. AI-agent co-author trailer -> FAIL (delegated to check-attribution.mjs)
newrepo
commit_as 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id' 'lostsky1403' 'moch22188ti@student.nurulfikri.ac.id' 'Co-Authored-By: Claude 4.7 <noreply@anthropic.com>'
assert 'AI co-author trailer' 1 "$(run_gate strict)"

# 11. Unresolvable range -> FAIL with exit 2 (missing prerequisite is never a pass)
newrepo
node "$SCRIPT" --range 'no-such-ref..HEAD' --mode strict >"$OUT" 2>&1
assert 'unresolvable range is exit 2' 2 $?

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
printf 'PASS: COMMIT_IDENTITY_POLICY\n'
