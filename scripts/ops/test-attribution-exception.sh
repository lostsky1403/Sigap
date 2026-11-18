#!/bin/sh
# test-attribution-exception.sh — proves the historical attribution exception is
# exact-SHA scoped and that new placeholder identities are rejected.
#
# Uses synthetic identities only; no secret values are printed or required.
# Exit 0 iff every case behaves as expected.

set -u
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)
GATE="$REPO_ROOT/scripts/check-commit-identity.mjs"

PASS=0; FAIL=0
assert() { # name expected actual
	if [ "$2" -eq "$3" ]; then PASS=$((PASS + 1)); printf 'PASS  %s (exit %s)\n' "$1" "$3";
	else FAIL=$((FAIL + 1)); printf 'FAIL  %s: expected %s, got %s\n' "$1" "$2" "$3"; fi
}

newrepo() {
	TMP=$(mktemp -d 2>/dev/null || echo "/tmp/sigap-attr-$$")
	git init -q "$TMP"
	cd "$TMP" || exit 1
	git config user.name 'lostsky1403'
	git config user.email 'moch22188ti@student.nurulfikri.ac.id'
	printf 'base\n' > f.txt; git add f.txt; git commit -q -m 'base'
}

run_gate() { # mode [env assignments...]
	mode=$1; shift
	env "$@" node "$GATE" --range 'HEAD^!' --mode "$mode" --branch feat/sigap-selfhosted-auth > "$OUT" 2>&1
	echo $?
}

# The 19 exact SHAs, used only by the "historical PASS" check below.
LEGACY=$(printf '%s\n' \
ffbd96dc3898d3383f3318d1c8fbe06992013073 f2621cf0e2286fe815942cd06b9808e911cd6866 \
62ad3fd40592801c6a08b3433325efb7a31e77ee 70c6f9fc746c1cc347bd111dde9404f548b0f2fe \
2db86fbe708c372d238ef2e51914596c60843190 ac53647b7405086770c7031a7e66f73e063a1d79 \
bc7abfca2a7739c00541d5dfcfc3d74f6e60194b 41b11deae833fffbd20da698af314cbddc3ce0ea \
5d092ad93c96f0273c7d85d77b525d14bfbb1d2d 37c1b130060ab4c3f7b504ca9abb8f6cc097a546 \
400e1285153759faed957aa45068e2c7a300c0cf c3f08ed3a867a9a084a8c7172a74faa1898dc828 \
8da750c7b076b290033a95cbadc8c9299fd11438 aa2c2b1044644ee216385f6ea485aa12d9065286 \
58dfafacfaa996827c483701a101e24b1d725dbf dff1cca33faebd63f1dde3f04cc72abb0d95efa4 \
6b1a572bd3156a6f24111d97f19ec18d1b10adc9 cdfdbbb4139995cdf2a32680b758d031e2ab0228 \
90c73b784bd88a4f1ccb53b22f80989c2fe46532 | tr '\n' ' ')

# 1. The verified historical range passes with exact SHAs grandfathered.
OUT="$REPO_ROOT/.attr-test.log"
cd "$REPO_ROOT" || exit 1
env SIGAP_LEGACY_AUTHOR_COMMITS="$LEGACY" node "$GATE" \
  --range '4813de64fcbac89ae9ce419f003a9120177c6867..HEAD' \
  --mode strict --branch feat/sigap-selfhosted-auth > "$OUT" 2>&1
assert 'historical 19 grandfathered by exact SHA' 0 $?
rm -f "$OUT"

# 2. A NEW SIGAP Design author must fail even with the legacy email in env.
newrepo; OUT="$TMP/out"
GIT_AUTHOR_NAME='SIGAP Design' GIT_AUTHOR_EMAIL='design@sigap.local' \
GIT_COMMITTER_NAME='lostsky1403' GIT_COMMITTER_EMAIL='moch22188ti@student.nurulfikri.ac.id' \
git commit -q --allow-empty -m 'new design placeholder'
assert 'new SIGAP Design author rejected' 1 "$(run_gate strict SIGAP_LEGACY_AUTHOR_COMMITS="$LEGACY")"

# 3. A NEW SIGAP Impl author must fail.
newrepo; OUT="$TMP/out"
GIT_AUTHOR_NAME='SIGAP Impl' GIT_AUTHOR_EMAIL='impl@sigap.local' \
GIT_COMMITTER_NAME='lostsky1403' GIT_COMMITTER_EMAIL='moch22188ti@student.nurulfikri.ac.id' \
git commit -q --allow-empty -m 'new impl placeholder'
assert 'new SIGAP Impl author rejected' 1 "$(run_gate strict SIGAP_LEGACY_AUTHOR_COMMITS="$LEGACY")"

# 4. sigap-ops must fail.
newrepo; OUT="$TMP/out"
GIT_AUTHOR_NAME='sigap-ops' GIT_AUTHOR_EMAIL='ops@local' \
GIT_COMMITTER_NAME='sigap-ops' GIT_COMMITTER_EMAIL='ops@local' \
git commit -q --allow-empty -m 'sigap-ops'
assert 'sigap-ops rejected' 1 "$(run_gate strict SIGAP_LEGACY_AUTHOR_COMMITS="$LEGACY")"

# 5. Legitimate maintainer identity passes.
newrepo; OUT="$TMP/out"
git commit -q --allow-empty -m 'maintainer commit'
assert 'maintainer commit accepted' 0 "$(run_gate strict SIGAP_LEGACY_AUTHOR_COMMITS="$LEGACY")"

# 6. Legitimate external fork contributor passes in permissive mode.
newrepo; OUT="$TMP/out"
GIT_AUTHOR_NAME='External Human' GIT_AUTHOR_EMAIL='contributor@example.org' \
GIT_COMMITTER_NAME='External Human' GIT_COMMITTER_EMAIL='contributor@example.org' \
git commit -q --allow-empty -m 'fork contribution'
assert 'fork contributor accepted in permissive mode' 0 "$(node "$GATE" --range 'HEAD^!' --mode permissive --branch fix/external > "$OUT" 2>&1; echo $?)"

# 7. Unauthorized AI trailer fails (delegated gate stays active).
newrepo; OUT="$TMP/out"
git commit -q --allow-empty -m 'ai trailer' -m 'Co-Authored-By: Claude 4.7 <noreply@anthropic.com>'
assert 'AI co-author trailer rejected' 1 "$(run_gate strict SIGAP_LEGACY_AUTHOR_COMMITS="$LEGACY")"

# 8. Genuine human co-author passes.
newrepo; OUT="$TMP/out"
git commit -q --allow-empty -m 'human co-author' -m 'Co-authored-by: Real Person <real@example.com>'
assert 'human co-author accepted' 0 "$(run_gate strict SIGAP_LEGACY_AUTHOR_COMMITS="$LEGACY")"

printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ] || exit 1
printf 'PASS: ATTRIBUTION_EXCEPTION_POLICY\n'
