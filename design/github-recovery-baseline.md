# GitHub recovery baseline — backup + restore drill (`lostsky1403/Sigap`)

**Captured:** 2026-11-11, mirror + bundle created 13:10 +07:00; verification, drill and live re-checks 13:35–13:55 +07:00 (worktree tip re-checked at 13:51: still `a5423ec`).
**Repository:** `https://github.com/lostsky1403/Sigap.git` (public, default `main` @ `b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3`, 0 forks, 0 releases).
**Backup location:** `%USERPROFILE%\sigap-attribution-backup\` — **DURABLE, outside both checkouts** (user-profile ACL; not subject to TEMP purging). Originally written to `%TEMP%\sigap-attribution-backup\`; relocated and **re-verified at the durable path**: `sha256sum -c SHA256SUMS.txt` → **55/55 OK**, mirror `git fsck --full` exit 0, and the restore drill re-run there from the bundle → **52/52 refs, fsck exit 0 — PASS**. Contents: `sigap-mirror.git` (remote snapshot, 120/120 refs identical to live), `sigap-local-all.bundle` (local snapshot, 19,961,112 B), `refs-manifest-2026-11-11.txt`, `SHA256SUMS.txt`, `evidence\` (52 raw captures).
**Status:** mirror == live remote (0 ref drift, fsck clean); bundle self-contained (git-verified); restore drill **PASS — 52/52 refs SHA-identical**; nothing was pushed, fetched-to, or rewritten on the remote.

---

## 0. Summary

| item | value |
|---|---|
| Remote ref lines (`git ls-remote origin`) | **125** = HEAD + **120 refs** (23 branches + 93 `refs/pull/*` + 4 tags) + 4 peeled-tag lines |

**Arithmetic reconciled:** 125 lines - 1 `HEAD` - 4 peeled `^{}` lines = the **120 refs**. Nothing is unaccounted, and the mirror holds exactly those 120 (0 missing, 0 extra, 0 SHA mismatch).
| Mirror (`git clone --mirror`, 13:10) | **120 refs, identical to live** (0 missing, 0 extra, 0 SHA mismatch); `git fsck --full` exit 0; HEAD = `refs/heads/main` @ `b96c5174` |
| Mirror size / pack SHA-256 | dir **2,812,734 B**; pack `pack-60b7646838d365ab6739733d505ccb42a60d95bb.pack` **2,654,156 B**, sha256 `fb46b1fb465c7ab3b6ade7a85974f644fb2379cc8bbc126da0363b25d3f5826e` |
| Bundle (`git bundle create --all`, 13:10) | **19,961,112 B**, sha256 `eff7eaf5a523df80cbd2f2db4081000eaa0dd156b3da1ddb0ab8218a478274d9`; 54 advertised entries = **52 refs** + `HEAD` + `main-worktree/HEAD`; `git bundle verify` → **"The bundle records a complete history."** |
| Restore drill | bare repo rebuilt from the bundle: **52/52 refs SHA-identical → PASS**, `fsck --full` exit 0; plain `git clone` from the bundle: exit 0, HEAD `a5423ec`, `fsck` exit 0 |
| PRs | **93 total, all MERGED, 0 open** (#1–#93); all authored by `lostsky1403`; 93 `refs/pull/*` retained in the mirror |
| Signatures on the 32 AI-trailer commits | **15 verified** (GitHub web-flow, reason `valid`) / **17 unsigned** |
| Local work | 22 branches, 2 worktrees (one shared object DB), 1 stash, 2 `refs/trae_merge/*`, 23 remote-tracking refs, 4 tags = 52 refs; **107 commits unpublished** (not on any remote ref) |
| Manifest file | `%TEMP%\sigap-attribution-backup\refs-manifest-2026-11-11.txt` (368 lines), sha256 `7c974f5dbbd2d812da7985e32c5f876ee311e6f39ec8a2814b11da6d7f1bd237` |

---

## 1. Remote ref inventory — A (verbatim `git ls-remote origin`, 125 lines)

```text
b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3	HEAD
f614805cfddc5e23239d81c10c515106da7439d0	refs/heads/ci/bucket-lock-drill
1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3	refs/heads/ci/gate-a-verified
05e19616401003ae669aa0e6870b690489e60de1	refs/heads/ci/lock-drill-refusal-reason
092b1ed86486d15d88472d93deb5de25fb348948	refs/heads/ci/ops-suite-baseline
98470f020df843990d2f29d2b728d8511c6e90b2	refs/heads/ci/r2-list-objects-fix
c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e	refs/heads/ci/remove-gitleaks-allowlist
d7404374734a3783224656e37a9714b22c3385af	refs/heads/ci/sigap-88-validation
73955d694ec7cd1447db169236aa94c90e536327	refs/heads/ci/sigap-r2-activation
704633cc7f317b9fc3cbe1189bd41f235f4d6d57	refs/heads/design/ui-ux-overhaul
f449d46033287f613d0d391d741bb3fec3673c64	refs/heads/docs/audit-701-production-proof
f8b4ef852597db742358a954fc67a85a3d0d88a7	refs/heads/feat/prod-api-port-override
fbd60b24207ab37f81f4ac626ba8cf8a5e3d93a6	refs/heads/feat/web-supabase-auth
919c8a797b01ab0890e781f3d0977adabd370e82	refs/heads/fix/auth-form-actions
68852794f263ec89068bbb4580e090f392afc975	refs/heads/fix/backup-postgres-pg16-client
6d90116c249657cd33b5cf02e65248d14a43d879	refs/heads/fix/backup-postgres-server-version-gate
b7aff840981e6521ff9ccaaa98d01265727a156b	refs/heads/fix/compose-auth-env-passthrough
072b1b1a082a040a3f72fbc328e37671bf5c58d2	refs/heads/fix/grpc-readyz-idle-connect
fb5204a712e08b1703759f4ffa80fdeb81c569f9	refs/heads/fix/prod-localhost-binds
2595bc327059bf83c34ff915d49cf385a075d68f	refs/heads/fix/prod-ports-override-replace
795aec95e03978bb25225ffa7fc9d193814d92e5	refs/heads/fix/request-id-propagation
c86e215322f9ee9f4eabe64b3a59860d2614697d	refs/heads/fix/web-supabase-ws-transport
b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3	refs/heads/main
9ec6c52cbe439df8a3295975cf3475e727082dac	refs/heads/ops/release-readiness-hardening
da2ac07f6cfb60ef066385ceae8b514912f0e822	refs/pull/1/head
10233d5af937819d8e2fb2af4c8ae42094c9ba5f	refs/pull/10/head
10233d5af937819d8e2fb2af4c8ae42094c9ba5f	refs/pull/11/head
283c6a7b7af9b876450a608e8619d0522a71da64	refs/pull/12/head
c7a92b74d3594b79cf0b9403169e951385d55c64	refs/pull/13/head
a7bad717fd3a96bbabe550eea80e27517e5e8513	refs/pull/14/head
71dc0bc51a6caeeab2b79bea6e057f1fbb1f370f	refs/pull/15/head
801e6805c5c92c37d8510649a99c9abdec4b65ea	refs/pull/16/head
f12f52f02c9392f809688be3a77eea3f12b7f5ed	refs/pull/17/head
164c1281897525d4025b485308c8f2d2b2df1f6b	refs/pull/18/head
63fa42a5fd1c307e7c4cdb1793034b3b2c73a7cf	refs/pull/19/head
a27c8ba1fe301199686be6a418df9e9ee1927e69	refs/pull/2/head
8a11e6d94807c02dceb81ecb1b32301f85dba780	refs/pull/20/head
66b8aec6b3293ea36dc9093aab31a3c626d95cb1	refs/pull/21/head
8971475b9f0de08e69504c9869d6241b7c872621	refs/pull/22/head
7fc4bde6c18901c8a5ba867036b64e9517927ae6	refs/pull/23/head
ece41bb912b76f50346b40f8674b5da5d52d5550	refs/pull/24/head
92133dc46e4a500b536e521dbef3abc393294a8f	refs/pull/25/head
f5aec54baa648e78888f260ad8bbd754fdfe2ff9	refs/pull/26/head
e46bcd75b1fdb673b52b432a642d1a12ca1d5cae	refs/pull/27/head
768459a82e8aad2af0f6e9844050217536b0d482	refs/pull/28/head
c4a0298bd17c10b34eb78a8144accabedfb6d223	refs/pull/29/head
0ddd01bbfb9c821d0295bbf30a26d8409d0b12de	refs/pull/3/head
297ee1d318f7f496ff0c6ca13c35c7d0faea5330	refs/pull/30/head
d8dcbbeec8c2eed2306ed53c7583a735b00d9e5b	refs/pull/31/head
c56f4838006b37292472533ed89b0dadc1c51cbb	refs/pull/32/head
de373897914def1b83c194c9c4e49667e0dcb626	refs/pull/33/head
95143456a569ec2f0e538c42c499dcc2867fd576	refs/pull/34/head
8fc64553f3121512e52b6f9f64f5be184ef998e1	refs/pull/35/head
bc4693e8ad80906456fa77bcc59469f1c2e7a4f5	refs/pull/36/head
7e23052700d3f8baae8bcad32b31d40d1839145b	refs/pull/37/head
d06f939670b0ccb37a83f541756b7f28ae3753d2	refs/pull/38/head
a929ba9582bd48d546a7a2fa9090f8866b3488d2	refs/pull/39/head
e7d28943d427d13966b2a1650761bb31fde09190	refs/pull/4/head
d37f8fed3e96f884ac181cb7de23086af33bd04b	refs/pull/40/head
f5bb72175f535c63016cc18450b22570998b587d	refs/pull/41/head
1745e4d33bd845c7fbb344da0fdfb2406bd4f24b	refs/pull/42/head
6ae8c5c83f8444f3c984658e50478f9884eee788	refs/pull/43/head
4877a6bd630ac873ac9a32caa1bcd5341045343c	refs/pull/44/head
018eaf25660757bfa8c455bdb9db838c9d800eff	refs/pull/45/head
81eb966f239f76e4296bdc4e4d348d892ffa22fc	refs/pull/46/head
143bee579beeb9d528b73f0d2b0505f371c70d87	refs/pull/47/head
b0635fab31ff284c678235e0f7e2bef059a997a2	refs/pull/48/head
395db605bff8d9ae5b4abade6e80b9002e518a41	refs/pull/49/head
a63b29ace80b3198c08465b6e33f0f348006abc2	refs/pull/5/head
510a915c6ef7d1a0fbf1132c34b83f509c854a3c	refs/pull/50/head
87c3830dcfcf1b240fc5fe3938c066159a9e8a0a	refs/pull/51/head
c04ad319837fcfd8cd366c318c661b2c49f32443	refs/pull/52/head
2b8fc1e86f834d0559f77bef2e411341ab89ed8e	refs/pull/53/head
763c01535ccb6d12c9fbfc749eca4d891491f1aa	refs/pull/54/head
ee8db8d48de83d3724f8ba620bdd978ef1b17b99	refs/pull/55/head
f5b488421f21d72940469400aa1998b6a20c5812	refs/pull/56/head
099c57103a0ac2986063925f9c6de2be9a2b6a3d	refs/pull/57/head
45c60779fe1ebbc47b56807955de62516aee7bef	refs/pull/58/head
a2d20c1350720ed0636cc79e6691307ba0417f81	refs/pull/59/head
13f207f4bbd064953f16502ef1953c6885709aa3	refs/pull/6/head
36c02fc1191394bd30fac84ccb304a6486dae91f	refs/pull/60/head
dcdec65a65e0f6d75810e8f821d9e6c6ce59f328	refs/pull/61/head
1d4a8b6069aab49e5edf5c1c6b1ff6161b68ae16	refs/pull/62/head
795aec95e03978bb25225ffa7fc9d193814d92e5	refs/pull/63/head
d470ff7017bbbcca71b1fa0454e826e6ee4a9d16	refs/pull/64/head
814a4a048535fe85fbcb2df1872fa3b6dac45c16	refs/pull/65/head
2592bfa3112dc6c67b22ae2a5d20c4362bd238f1	refs/pull/66/head
1460ac8b90aff7dbac1e0170c036000d5dca5411	refs/pull/67/head
e12cc327f4e94ef6e0101dae94b99ab9da09b969	refs/pull/68/head
03120eb85a26bb94883f63439fffedd5647d928e	refs/pull/69/head
3d8d050d73a234228323843b1d77574eb66b844e	refs/pull/7/head
7d4d3a2ae1cf4f81b251088fa4f3d95b11dc2b90	refs/pull/70/head
d6be4b7892447f9de48e4b47c431ab1947bcc333	refs/pull/71/head
68852794f263ec89068bbb4580e090f392afc975	refs/pull/72/head
6d90116c249657cd33b5cf02e65248d14a43d879	refs/pull/73/head
f449d46033287f613d0d391d741bb3fec3673c64	refs/pull/74/head
f8b4ef852597db742358a954fc67a85a3d0d88a7	refs/pull/75/head
2595bc327059bf83c34ff915d49cf385a075d68f	refs/pull/76/head
b7aff840981e6521ff9ccaaa98d01265727a156b	refs/pull/77/head
fbd60b24207ab37f81f4ac626ba8cf8a5e3d93a6	refs/pull/78/head
c86e215322f9ee9f4eabe64b3a59860d2614697d	refs/pull/79/head
d00aa17e5cebcb5f7a5991ae6dbc256a715fb0ec	refs/pull/8/head
072b1b1a082a040a3f72fbc328e37671bf5c58d2	refs/pull/80/head
fb5204a712e08b1703759f4ffa80fdeb81c569f9	refs/pull/81/head
919c8a797b01ab0890e781f3d0977adabd370e82	refs/pull/82/head
739032986635c76eb4f7bd542505d75da5395d90	refs/pull/83/head
d7404374734a3783224656e37a9714b22c3385af	refs/pull/84/head
73955d694ec7cd1447db169236aa94c90e536327	refs/pull/85/head
092b1ed86486d15d88472d93deb5de25fb348948	refs/pull/86/head
c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e	refs/pull/87/head
05e19616401003ae669aa0e6870b690489e60de1	refs/pull/88/head
1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3	refs/pull/89/head
137aa7e04dff73b4ea648b4c2a91f8e208ddd4fa	refs/pull/9/head
f614805cfddc5e23239d81c10c515106da7439d0	refs/pull/90/head
98470f020df843990d2f29d2b728d8511c6e90b2	refs/pull/91/head
9ec6c52cbe439df8a3295975cf3475e727082dac	refs/pull/92/head
704633cc7f317b9fc3cbe1189bd41f235f4d6d57	refs/pull/93/head
62754ed83d97e5d887ac92a7893b95f8f5a95851	refs/tags/demo-handoff-green-2026-07-10
0f454996dd5c7974009e721d221c805942569866	refs/tags/demo-handoff-green-2026-07-10^{}
001057de41fd4678983da243caff791f1a8cfc71	refs/tags/frontend-demo-green-2026-07-10
7ec14da1a9628b985f896d720c54199ff4460695	refs/tags/frontend-demo-green-2026-07-10^{}
fc0df5c8778d4775336b8bd3f13060e21814bff0	refs/tags/local-demo-green-2026-07-09
9339571979c44dd74d8b735dfb0043f0c4b7e3a8	refs/tags/local-demo-green-2026-07-09^{}
3380ec26b303e4f1525185a32693b5284c7acb37	refs/tags/v1.0.0-beta
13c59e7a03dcca019e9125ea6051ace4344e1177	refs/tags/v1.0.0-beta^{}
```

Breakdown: 1 `HEAD` line (= `refs/heads/main`), 23 `refs/heads/*`, 93 `refs/pull/*`, 4 `refs/tags/*` (annotated — each also appears as a `^{}` peeled line), total 125 lines. Saved verbatim as `evidence/refs-live-2026-11-11.txt`.

---

## 2. Mirror snapshot — B (the remote, preserved)

```bash
git clone --mirror https://github.com/lostsky1403/Sigap.git "$TEMP/sigap-attribution-backup/sigap-mirror.git"
git --git-dir="$TEMP/sigap-attribution-backup/sigap-mirror.git" for-each-ref --format="%(objectname) %(refname)"
```

Verification (2026-11-11 ~13:40): the mirror's 120 refs compared against a fresh live `git ls-remote origin` — **0 refs missing, 0 extra, 0 SHA mismatches**. `git fsck --full` → exit 0 (no errors; 0 loose objects, 3,853 objects in 1 pack). Mirror HEAD = `refs/heads/main` @ `b96c5174`. Full listing: `evidence/mirror-refs-live-2026-11-11.txt`.

This is the only artifact that preserves the **93 `refs/pull/*` refs** — they pin every PR's pre-squash head; the bundle does not contain them.

---

## 3. Local bundle — C (`git bundle create --all`, self-contained)

```bash
git -C F:/sigap-design-v2 bundle create "$TEMP/sigap-attribution-backup/sigap-local-all.bundle" --all
git -C F:/sigap-design-v2 bundle verify "$TEMP/sigap-attribution-backup/sigap-local-all.bundle"
# -> "The bundle records a complete history." / "... is okay"
```

54 advertised entries (52 refs + 2 HEAD pseudo-refs):

```text
a5423ec35b197395d4d506269d6dd26a1b56b17f HEAD
704633cc7f317b9fc3cbe1189bd41f235f4d6d57 main-worktree/HEAD
f614805cfddc5e23239d81c10c515106da7439d0 refs/heads/ci/bucket-lock-drill
1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3 refs/heads/ci/gate-a-verified
05e19616401003ae669aa0e6870b690489e60de1 refs/heads/ci/lock-drill-refusal-reason
092b1ed86486d15d88472d93deb5de25fb348948 refs/heads/ci/ops-suite-baseline
98470f020df843990d2f29d2b728d8511c6e90b2 refs/heads/ci/r2-list-objects-fix
c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e refs/heads/ci/remove-gitleaks-allowlist
d7404374734a3783224656e37a9714b22c3385af refs/heads/ci/sigap-88-validation
73955d694ec7cd1447db169236aa94c90e536327 refs/heads/ci/sigap-r2-activation
4e470a5332b6db037ed68b2e13873f4a1ff31e4a refs/heads/design/sigap-v2-prototype
d7af334d381a48fbd1054060e5eaffcc6c4521a5 refs/heads/design/ui-ux-overhaul
522fbc48550d357f1ab162a6b316fd4e91c611b7 refs/heads/feat/sigap-regional-pilot
a5423ec35b197395d4d506269d6dd26a1b56b17f refs/heads/feat/sigap-selfhosted-auth
e163189490aeea71ef798ce610a2a455d313de8d refs/heads/feat/sigap-v2-implementation
919c8a797b01ab0890e781f3d0977adabd370e82 refs/heads/fix/auth-form-actions
072b1b1a082a040a3f72fbc328e37671bf5c58d2 refs/heads/fix/grpc-readyz-idle-connect
fb5204a712e08b1703759f4ffa80fdeb81c569f9 refs/heads/fix/prod-localhost-binds
795aec95e03978bb25225ffa7fc9d193814d92e5 refs/heads/fix/request-id-propagation
c86e215322f9ee9f4eabe64b3a59860d2614697d refs/heads/fix/web-supabase-ws-transport
704633cc7f317b9fc3cbe1189bd41f235f4d6d57 refs/heads/local/r2-drill
87f218efe71deca217fbc6c46668e3b0be499503 refs/heads/main
9ec6c52cbe439df8a3295975cf3475e727082dac refs/heads/ops/release-readiness-hardening
3f0f7689d8687d067dc18bd15a6a534317dfe1d8 refs/heads/safety/pre-main-sync-3f0f768
f614805cfddc5e23239d81c10c515106da7439d0 refs/remotes/origin/ci/bucket-lock-drill
1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3 refs/remotes/origin/ci/gate-a-verified
05e19616401003ae669aa0e6870b690489e60de1 refs/remotes/origin/ci/lock-drill-refusal-reason
092b1ed86486d15d88472d93deb5de25fb348948 refs/remotes/origin/ci/ops-suite-baseline
98470f020df843990d2f29d2b728d8511c6e90b2 refs/remotes/origin/ci/r2-list-objects-fix
c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e refs/remotes/origin/ci/remove-gitleaks-allowlist
d7404374734a3783224656e37a9714b22c3385af refs/remotes/origin/ci/sigap-88-validation
73955d694ec7cd1447db169236aa94c90e536327 refs/remotes/origin/ci/sigap-r2-activation
704633cc7f317b9fc3cbe1189bd41f235f4d6d57 refs/remotes/origin/design/ui-ux-overhaul
f449d46033287f613d0d391d741bb3fec3673c64 refs/remotes/origin/docs/audit-701-production-proof
f8b4ef852597db742358a954fc67a85a3d0d88a7 refs/remotes/origin/feat/prod-api-port-override
fbd60b24207ab37f81f4ac626ba8cf8a5e3d93a6 refs/remotes/origin/feat/web-supabase-auth
919c8a797b01ab0890e781f3d0977adabd370e82 refs/remotes/origin/fix/auth-form-actions
68852794f263ec89068bbb4580e090f392afc975 refs/remotes/origin/fix/backup-postgres-pg16-client
6d90116c249657cd33b5cf02e65248d14a43d879 refs/remotes/origin/fix/backup-postgres-server-version-gate
b7aff840981e6521ff9ccaaa98d01265727a156b refs/remotes/origin/fix/compose-auth-env-passthrough
072b1b1a082a040a3f72fbc328e37671bf5c58d2 refs/remotes/origin/fix/grpc-readyz-idle-connect
fb5204a712e08b1703759f4ffa80fdeb81c569f9 refs/remotes/origin/fix/prod-localhost-binds
2595bc327059bf83c34ff915d49cf385a075d68f refs/remotes/origin/fix/prod-ports-override-replace
795aec95e03978bb25225ffa7fc9d193814d92e5 refs/remotes/origin/fix/request-id-propagation
c86e215322f9ee9f4eabe64b3a59860d2614697d refs/remotes/origin/fix/web-supabase-ws-transport
b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 refs/remotes/origin/main
9ec6c52cbe439df8a3295975cf3475e727082dac refs/remotes/origin/ops/release-readiness-hardening
7419094c3fe4ce1b8b50270530f2fa4a9eea66f6 refs/stash
62754ed83d97e5d887ac92a7893b95f8f5a95851 refs/tags/demo-handoff-green-2026-07-10
001057de41fd4678983da243caff791f1a8cfc71 refs/tags/frontend-demo-green-2026-07-10
fc0df5c8778d4775336b8bd3f13060e21814bff0 refs/tags/local-demo-green-2026-07-09
3380ec26b303e4f1525185a32693b5284c7acb37 refs/tags/v1.0.0-beta
0f454996dd5c7974009e721d221c805942569866 refs/trae_merge/worktree/feat-demo-cleanup-stabilization-1ZK8ui/f4f7c9add59c93e79b195fd37373707bfe6a0e28-chore/demo-data-cleanup/0f454996dd5c7974009e721d221c805942569866/mainHash
f4f7c9add59c93e79b195fd37373707bfe6a0e28 refs/trae_merge/worktree/feat-demo-cleanup-stabilization-1ZK8ui/f4f7c9add59c93e79b195fd37373707bfe6a0e28-chore/demo-data-cleanup/0f454996dd5c7974009e721d221c805942569866/worktreeHash
```

Notes: `HEAD` = `a5423ec` (the `F:/sigap-design-v2` worktree tip, `feat/sigap-selfhosted-auth`); `main-worktree/HEAD` = `704633c` (the main worktree `F:/Sehat-Hub`, branch `local/r2-drill`). Both are recorded so the snapshot survives whichever checkout is restored. Bundle byte size **19,961,112**, sha256 `eff7eaf5a523df80cbd2f2db4081000eaa0dd156b3da1ddb0ab8218a478274d9`. Saved listing: `evidence/bundle-heads-2026-11-11.txt`.

---

## 4. Restore drill — D (RUN, not described)

### 4.1 Exact-ref drill (bare; the acceptance path)

```bash
git init --bare "$TEMP/sigap-restore-drill-bare.git"
git -C "$TEMP/sigap-restore-drill-bare.git" fetch "$TEMP/sigap-attribution-backup/sigap-local-all.bundle" "+refs/*:refs/*"
git -C "$TEMP/sigap-restore-drill-bare.git" fsck --full           # exit 0
git -C "$TEMP/sigap-restore-drill-bare.git" symbolic-ref HEAD refs/heads/feat/sigap-selfhosted-auth
```

**Result:** fetch exit 0; 52 refs created; every one compared against the bundle's advertised refs (excluding the `HEAD`/`main-worktree/HEAD` pseudo-refs): **missing: none · extra: none · SHA mismatches: none → PASS (52/52)**. `fsck --full` exit 0. Full ref set: `evidence/drill-bare-refs.txt`.

### 4.2 Plain clone drill (the everyday path)

```bash
git clone "$TEMP/sigap-attribution-backup/sigap-local-all.bundle" "$TEMP/sigap-restore-drill-clone"
```

**Result:** clone exit 0 → HEAD `refs/heads/feat/sigap-selfhosted-auth` @ `a5423ec35b197395d4d506269d6dd26a1b56b17f` (the currently unpublished work); working tree checked out; pack 5,306 objects / 19.17 MiB; 28 refs materialise (22 heads + 4 tags + 2 origin remotes); `git fsck --full` exit 0 with **9 dangling commits** — objects present in the pack but not referenced by the clone's reduced ref namespace (they are reachable in the exact-ref drill and in the bundle itself):

```text
68852794f263ec89068bbb4580e090f392afc975  f449d46033287f613d0d391d741bb3fec3673c64  6d90116c249657cd33b5cf02e65248d14a43d879
2595bc327059bf83c34ff915d49cf385a075d68f  fbd60b24207ab37f81f4ac626ba8cf8a5e3d93a6  7419094c3fe4ce1b8b50270530f2fa4a9eea66f6
b7aff840981e6521ff9ccaaa98d01265727a156b  f8b4ef852597db742358a954fc67a85a3d0d88a7  f4f7c9add59c93e79b195fd37373707bfe6a0e28
```

### 4.3 Drill verdict

| drill | refs compared | result | fsck |
|---|---|---|---|
| bare, `+refs/*:refs/*` | 52/52 | **PASS** | exit 0 |
| plain clone | 28 refs materialised; HEAD = unpublished tip | **PASS** | exit 0 (9 dangling, expected) |
| mirror | 120 refs vs live remote | **PASS** | exit 0 |

Drill artifacts (disposable): `%TEMP%\sigap-restore-drill-bare.git`, `%TEMP%\sigap-restore-drill-clone`.

---

## 5. Tags — E (4, all annotated: tag object → commit)

| tag | tag object SHA | peeled commit SHA |
|---|---|---|
| `demo-handoff-green-2026-07-10` | `62754ed83d97e5d887ac92a7893b95f8f5a95851` | `0f454996dd5c7974009e721d221c805942569866` |
| `frontend-demo-green-2026-07-10` | `001057de41fd4678983da243caff791f1a8cfc71` | `7ec14da1a9628b985f896d720c54199ff4460695` |
| `local-demo-green-2026-07-09` | `fc0df5c8778d4775336b8bd3f13060e21814bff0` | `9339571979c44dd74d8b735dfb0043f0c4b7e3a8` |
| `v1.0.0-beta` | `3380ec26b303e4f1525185a32693b5284c7acb37` | `13c59e7a03dcca019e9125ea6051ace4344e1177` |

Live API check (`api-tags-live.json`): all four present with the same peeled commits. All four are annotated; the peeled lines are the 4 `^{}` lines in section 1. Note: `v1.0.0-beta` is the only ref that a message-only trailer rewrite would leave untouched (its commit predates every trailer commit); the other three tags move.

---

## 6. Pull-request inventory — F (API, paged; `state=all`)

`GET /repos/lostsky1403/Sigap/pulls?state=all&per_page=100` returned **93 PRs in one page** (no further pages): **0 open, 93 closed, 93 merged** (`merged_at` set), numbers **#1–#93** contiguous, all authored by `lostsky1403`, 93 distinct `merge_commit_sha`. Raw: `evidence/api-pulls-stateall-live.json` (1.57 MB) and the earlier full fetch `api-pulls-all.json`.

| # | merged_at | merge_commit | head | title |
|---|---|---|---|---|
| 1 | 2026-06-19T20:29:18Z | `3807050a1a5a` | `da2ac07f6cfb` | Feat/first 10 prs security foundation |
| 2 | 2026-06-20T13:19:10Z | `b3653680d153` | `a27c8ba1fe30` | Feat/facility admin queue console |
| 3 | 2026-06-20T20:30:53Z | `1d2799f9bf32` | `0ddd01bbfb9c` | Feat/appointment scheduling checkin |
| 4 | 2026-06-20T20:34:58Z | `f6ef9dd2409a` | `e7d28943d427` | fix(web): resolve schedule modal accessibility warnings |
| 5 | 2026-06-21T07:19:44Z | `78c5e84f79de` | `a63b29ace80b` | Feat/demo readiness smoke suite |
| 6 | 2026-06-21T12:45:53Z | `310ceb08a4ee` | `13f207f4bbd0` | PR Autopilot Script |
| 7 | 2026-06-21T15:52:21Z | `596e71a24a0d` | `3d8d050d73a2` | Notification Outbox and Patient Communication Foundation |
| 8 | 2026-06-22T05:54:08Z | `e9b8b33678df` | `d00aa17e5ceb` | Notification Worker and Template Renderer |
| 9 | 2026-06-22T15:42:07Z | `7962f25b90a5` | `137aa7e04dff` | Notification Admin Monitoring |
| 10 | 2026-06-23T12:38:19Z | `9490d674ac6e` | `10233d5af937` | Notification Worker Operations Hardening |
| 11 | 2026-06-23T17:23:02Z | `81bdd8a40e7e` | `10233d5af937` | Notification Worker Operations Hardening |
| 12 | 2026-07-02T19:10:42Z | `d0288fee9076` | `283c6a7b7af9` | Notification E2E Smoke Script |
| 13 | 2026-07-03T11:55:52Z | `d528f7ab5100` | `c7a92b74d359` | Harden notification smoke runtime |
| 14 | 2026-07-03T17:50:21Z | `c3e7493040f2` | `a7bad717fd3a` | Add patient status portal foundation |
| 15 | 2026-07-03T18:30:47Z | `cd2904ff56b1` | `71dc0bc51a6c` | Harden patient portal status lookup |
| 16 | 2026-07-03T19:06:32Z | `1abc332628f0` | `801e6805c5c9` | Add local demo runbook |
| 17 | 2026-07-04T08:47:37Z | `5e20a43db75a` | `f12f52f02c93` | Fix local demo seed smoke |
| 18 | 2026-07-04T09:17:53Z | `c14bea408f91` | `164c12818975` | Harden PR checks retry |
| 19 | 2026-07-05T10:16:52Z | `f2d396ccc491` | `63fa42a5fd1c` | Harden local smoke determinism |
| 20 | 2026-07-05T14:04:12Z | `7bfbdbc8aa89` | `8a11e6d94807` | Fix dev smoke audit identity |
| 21 | 2026-07-05T19:08:24Z | `2aa9d1b22eb8` | `66b8aec6b329` | Make RBAC seed idempotent |
| 22 | 2026-07-05T20:23:12Z | `48ae641c46d1` | `8971475b9f0d` | Add full local demo smoke runner |
| 23 | 2026-07-06T07:54:56Z | `f5d86d7dd328` | `7fc4bde6c189` | Prefer canonical demo facility in smoke |
| 24 | 2026-07-06T08:35:00Z | `707063affe67` | `ece41bb912b7` | Document demo facility fallback |
| 25 | 2026-07-06T16:10:07Z | `a18caeff2160` | `92133dc46e4a` | Make dev facility seed idempotent |
| 26 | 2026-07-08T06:24:45Z | `0c741a4bc905` | `f5aec54baa64` | Persist dev queue fallback ticket |
| 27 | 2026-07-08T11:58:22Z | `de0b46b6c312` | `e46bcd75b1fd` | Document local demo runtime modes |
| 28 | 2026-07-08T15:18:28Z | `9339571979c4` | `768459a82e8a` | Improve admin queue demo responses |
| 29 | 2026-07-10T11:49:12Z | `7ec14da1a962` | `c4a0298bd17c` | Improve frontend demo flow |
| 30 | 2026-07-10T16:52:49Z | `0f454996dd5c` | `297ee1d318f7` | Add demo handoff checkpoint |
| 31 | 2026-08-22T18:22:09Z | `9ccd18f956f5` | `d8dcbbeec8c2` | Remove Trae automation artifacts |
| 32 | 2026-08-22T18:13:24Z | `9daa5cf3053e` | `c56f4838006b` | Refresh vulnerable dependencies |
| 33 | 2026-08-23T12:33:04Z | `1ca04261b40e` | `de373897914d` | Fix browser appointment and queue flows |
| 34 | 2026-08-23T14:00:28Z | `ef33ad05b5c1` | `95143456a569` | fix: propagate verify-only failures in pr-autopilot |
| 35 | 2026-08-23T14:29:29Z | `0c08ac679a14` | `8fc64553f312` | fix: improve local runtime bootstrap for bare-metal Windows dev |
| 36 | 2026-08-23T14:41:44Z | `55353d123249` | `bc4693e8ad80` | fix: remove remaining IPv6 doc references from smoke scripts |
| 37 | 2026-08-23T16:29:04Z | `c7550424bef6` | `7e23052700d3` | Polish final demo flow |
| 38 | 2026-08-23T17:54:16Z | `b47e07da6923` | `d06f939670b0` | Add final demo runbook |
| 39 | 2026-08-24T17:49:13Z | `b3751bfdba9b` | `a929ba9582bd` | Add production readiness audit |
| 40 | 2026-08-24T19:04:51Z | `a22313f572a7` | `d37f8fed3e96` | Guard dev-only capabilities by environment |
| 41 | 2026-08-24T19:26:53Z | `cfef087a494b` | `f5bb72175f53` | Require SIGAP_AUTH_MODE in compose and add boot banner |
| 42 | 2026-08-24T19:56:56Z | `c756edbada22` | `1745e4d33bd8` | Delete demo PHI handlers and remove SIGAP_ENABLE_DEMO_PHI |
| 43 | 2026-08-24T20:06:37Z | `bd512dfd680a` | `6ae8c5c83f84` | Require TLS termination confirmation outside local |
| 44 | 2026-08-24T20:23:47Z | `dcdb6430c6fb` | `4877a6bd630a` | Restrict CORS loopback exception to loopback deployments |
| 45 | 2026-08-24T20:31:23Z | `f0dd74437d2a` | `018eaf256607` | Remove hardcoded Access-Control-Allow-Origin: * from SSE endpoint |
| 46 | 2026-08-24T21:00:42Z | `30790d37b353` | `81eb966f239f` | Add security headers to API and web server |
| 47 | 2026-08-24T21:10:57Z | `142dc5d64b9a` | `143bee579bee` | Sanitize proxy headers for real client IP resolution |
| 48 | 2026-08-24T21:27:30Z | `0b8b7c294ab8` | `b0635fab31ff` | Public catalog endpoints and centralized web proxy auth |
| 49 | 2026-08-24T21:51:52Z | `48f68be15986` | `395db605bff8` | Make /readyz check database connectivity and surface audit state |
| 50 | 2026-08-24T22:01:34Z | `7de59522f26a` | `510a915c6ef7` | Harden HTTP server with timeouts, body limits, and graceful shutdown |
| 51 | 2026-08-24T22:40:37Z | `d84fed94a2ae` | `87c3830dcfcf` | feat(db): tracked migration runner with schema_migrations |
| 52 | 2026-08-24T22:52:28Z | `5f4e101f2e31` | `c04ad319837f` | docs: reconcile production readiness audit |
| 53 | 2026-08-24T23:18:04Z | `ce2df06b2d9f` | `2b8fc1e86f83` | fix(api): atomic check-in transitions and rate-limited attempts |
| 54 | 2026-08-25T13:17:01Z | `f622bbc47a3d` | `763c01535ccb` | fix(api): resolve JWT permissions from server-side RBAC |
| 55 | 2026-08-28T07:10:18Z | `06fec1f4ea7c` | `ee8db8d48de8` | fix(api): enforce facility-scoped authorization |
| 56 | 2026-08-28T12:41:38Z | `449f50995a6b` | `f5b488421f21` | docs: reconcile production audit after RBAC remediation |
| 57 | 2026-08-30T15:56:17Z | `9621559f12f4` | `099c57103a0a` | ci: run PostgreSQL-backed integration tests |
| 58 | 2026-08-30T15:43:58Z | `9e8794b9afd9` | `45c60779fe1e` | fix(api): prevent concurrent check-in race condition (AUDIT-1004) |
| 59 | 2026-08-30T16:40:18Z | `ad656eff1bdb` | `a2d20c135072` | docs: reconcile production audit after CI integration |
| 60 | 2026-08-30T17:13:15Z | `70f8d035227c` | `36c02fc11913` | fix(queue-engine): remove baked-in default database credentials |
| 61 | 2026-08-30T18:51:09Z | `b8b5b932bb8e` | `dcdec65a65e0` | fix(queue-engine): report missing database configuration clearly |
| 62 | 2026-08-30T20:32:19Z | `4af96882a261` | `1d4a8b6069aa` | docs: record AUDIT-801 closure after PR #60-#61 |
| 63 | 2026-09-01T17:18:08Z | `de15089b399d` | `795aec95e039` | fix(api): propagate request IDs through audit and logs |
| 64 | 2026-09-05T08:09:59Z | `05dd7da90e5d` | `d470ff7017bb` | docs: reconcile production audit after request ID remediation |
| 65 | 2026-09-06T13:21:13Z | `982e655f4d5a` | `814a4a048535` | fix(db): isolate demo seeds from schema changes and gate by environment |
| 66 | 2026-09-06T13:31:59Z | `cecc5cadcfaf` | `2592bfa3112d` | docs: reconcile audit after demo seed isolation |
| 67 | 2026-09-06T14:36:59Z | `da9f83bf64c9` | `1460ac8b90af` | ops: add production PostgreSQL backup and restore workflow |
| 68 | 2026-09-06T14:44:44Z | `3476555abed6` | `e12cc327f4e9` | docs: reconcile audit after postgres backup and restore |
| 69 | 2026-09-06T15:34:34Z | `e8219d56bf89` | `03120eb85a26` | docs: correct AUDIT-701 deployment-blocked status |
| 70 | 2026-09-07T06:53:33Z | `1037fb1527b7` | `7d4d3a2ae1cf` | fix(queue-engine): copy full protos tree for Docker build |
| 71 | 2026-09-07T07:15:35Z | `96a0dc9db2cd` | `d6be4b789244` | fix(queue-engine): mirror repo layout in Docker build context |
| 72 | 2026-09-07T14:23:46Z | `e9335ece6c2a` | `68852794f263` | fix(backup): resolve PostgreSQL 16 pg_dump/pg_restore explicitly |
| 73 | 2026-09-07T14:31:45Z | `b6c4d436a7c9` | `6d90116c2496` | fix(backup): fail fast when pg_dump is older than server |
| 74 | 2026-09-07T14:50:52Z | `4ac565ea92a5` | `f449d4603328` | docs: record production backup and restore proof |
| 75 | 2026-09-07T15:02:20Z | `ec77f2f7901c` | `f8b4ef852597` | feat(deploy): add production port override for shared VPS hosts |
| 76 | 2026-09-07T15:09:18Z | `1668097bce00` | `2595bc327059` | fix(deploy): replace api ports in prod override instead of merging |
| 77 | 2026-09-07T19:34:47Z | `672b9bc00a18` | `b7aff840981e` | fix(deploy): pass auth and database env to api container |
| 78 | 2026-09-07T21:05:27Z | `9c6f32d9fae5` | `fbd60b24207a` | feat(web): Supabase Auth login, register, logout with bearer forwarding |
| 79 | 2026-09-08T05:19:31Z | `06ee21b079bd` | `c86e215322f9` | fix(web): use Node 22 runtime for native WebSocket in Supabase SSR |
| 80 | 2026-09-08T05:40:55Z | `6d8ee63b47c7` | `072b1b1a082a` | fix(api): connect idle gRPC conn in readyz probe instead of flapping 503 |
| 81 | 2026-09-08T06:21:25Z | `1e0cfdba9388` | `fb5204a712e0` | fix(deploy): bind prod backend ports to loopback only |
| 82 | 2026-09-08T17:19:54Z | `ba2cb4cb67e9` | `919c8a797b01` | fix(web): wire auth forms to default actions so POST reaches handlers not 404 |
| 83 | 2026-09-12T08:25:24Z | `6d7f940b10be` | `739032986635` | deploy: env-driven production origin + shared edge routing |
| 84 | 2026-10-12T02:45:27Z | `d7404374734a` | `d7404374734a` | ci: validate SIGAP-88 isolated QA on GitHub-hosted runners |
| 85 | 2026-10-13T05:04:59Z | `3ba17b5f1243` | `73955d694ec7` | ci(ops): make the R2 validation gate reachable, and stop three tests passing vacuously |
| 86 | 2026-10-13T04:31:03Z | `5864945b93f1` | `092b1ed86486` | fix(ci): remediate the two pre-existing CI failures — a silently-skipping guard test and 13 govulncheck findings |
| 87 | 2026-10-13T06:04:11Z | `8f0370e803c4` | `c443ce0237a0` | chore: remove the historical gitleaks allowlist now that its justification is gone |
| 88 | 2026-10-13T07:23:45Z | `d7af334d381a` | `05e196164010` | docs(ops): make the Gate A and preservation handoff executable — drill refusal reason, relay prerequisite, stale gate records |
| 89 | 2026-10-13T08:02:44Z | `a66fc90441d6` | `1ba464bf9b3e` | docs(ops): record the operator-verified Gate A configuration, separating it from the retention-behaviour test |
| 90 | 2026-10-13T14:08:45Z | `8bc82d73a232` | `f614805cfddc` | ops(drill): ship the Bucket Lock retention drill as a PowerShell-native, tested procedure |
| 91 | 2026-10-13T17:19:39Z | `1228bab1655b` | `98470f020df8` | ops(drill): read the drill-prefix listing from Contents, not from KeyCount |
| 92 | 2026-10-14T04:49:32Z | `704633cc7f31` | `9ec6c52cbe43` | ops(release): resolve J11, harden the backup timer, and add the source-side preservation path |
| 93 | 2026-10-14T17:54:43Z | `b96c5174ff1c` | `704633cc7f31` | release: SIGAP UI/UX redesign → main (704633c) |

Recovery relevance: every merged PR is still represented on the remote by a `refs/pull/<n>/head` ref (93 of them, all in the mirror). Those pin pre-squash branch tips — the recovery asset for anything that would otherwise become unreachable after a force-push. They are read-only on GitHub (a `push --mirror` back to GitHub cannot write them).

---

## 7. Signature status of the 32 trailer commits — G

Source: `evidence/api-commit-verification.json` (GitHub `verification` object for all 32) + local `git %G?`. Live re-sample 2026-11-11: `a22313f` → `verified: true`, `reason: valid`, `verified_at 2026-08-24T19:04:51Z`; `030447b` → `verified: false`, `reason: unsigned`.

**15 verified** (all are GitHub web-flow squash-merges — PRs #40–#50, #67–#69, #87; `committer.login = web-flow`):

* `0b8b7c294ab8e6b7198fad9ade6b98ecc3fcc2a7` — feat: public catalog endpoints and centralized web proxy auth (#48)
* `142dc5d64b9a6fb63e499b8fef815e46a7931020` — fix: sanitize proxy headers for real client IP resolution (#47)
* `30790d37b3531b11e4c6287df642e1dd075fd95b` — feat: add security headers to API and web server (#46)
* `3476555abed65c73900bd53b7778c21a36c3a71b` — docs: reconcile audit after postgres backup and restore (#68)
* `48f68be1598649b13b69f284ca61408a78736022` — fix: make /readyz check database connectivity and surface audit state (#49)
* `7de59522f26a42f55dae36b7681d834feb58701e` — feat: harden HTTP server with timeouts, body limits, and graceful shutdown (#50)
* `8f0370e803c4afcc1131baa92c36396ceea2359a` — chore: remove the historical gitleaks allowlist (#87)
* `a22313f572a7450ee40f67c44e993d73115b5c03` — fix: guard dev-only capabilities by environment (#40)
* `bd512dfd680af10725823c5fd0e0f7da09647b0f` — fix: require TLS termination confirmation outside local environment (#43)
* `c756edbada220db880d54ab4bc49d8711a56802c` — fix: delete demo PHI handlers and remove SIGAP_ENABLE_DEMO_PHI (#42)
* `cfef087a494b9661d275efd9dd71e047b69e4e1c` — fix: require SIGAP_AUTH_MODE in compose and add boot banner (#41)
* `da9f83bf64c9fb97a2f3d522fda07a47dd3b44d1` — ops: add PostgreSQL backup and restore workflow (#67)
* `dcdb6430c6fb88ab82d0faf39bde4f094661918d` — fix: restrict CORS loopback exception to loopback deployments (#44)
* `e8219d56bf89f1c379530bd3fb00a41d6b686f6a` — docs: correct backup restore closure status (#69)
* `f0dd74437d2a25dc13d725a2effe857a57bb5804` — fix: remove hardcoded Access-Control-Allow-Origin: * from SSE endpoint (#45)

**17 unsigned** (`committer` = the owner; direct pushes / merge-PR #1 commits):

* `030447b5af557f15b279533f83d74ce537dcb6af` — chore: pre-termination backup
* `29e8c73dda822f589e2bddec7747ca6e0e9ab0d0` — chore: pre-termination backup
* `5d762f89101087cbb2fca144712874a6cb267558` — chore: pre-termination backup
* `7119c690af773d9e7970a16d3a3dec045da554a8` — chore: pre-termination backup
* `776d0c291c54b5b36fb8dac8402d9f8a6a01e7bf` — chore: pre-termination backup
* `7cf4505c13c60cf943f700d0cf12e52d8d3c7519` — fix(security): remove hardcoded Postgres password, parameterize secrets
* `8bf9120de809def2ac9f7970d8e1c3c99db0e856` — feat(api): route registry + deny-by-default 401 middleware
* `8e57eeea84990ac7facaffcccf896f041684ced1` — chore: pre-termination backup
* `908dd21718fd2d125ece9fc3f4c74a03057bd10e` — chore: pre-termination backup
* `a144eb8bb06e747c8db54ada699d2172b6256bb1` — fix(security): enforce Postgres TLS (sslmode=require) and run web as non-root
* `be7e1592c86809306098204042c5615bc57cdbeb` — fix(security): close open PHI routes behind a default-off gate
* `c179bdf7485dbf013e6220c9738f69188728bf58` — chore: pre-termination backup
* `cd89f10ab5c3e596f2371a99af6a249d80439aea` — chore: pre-termination backup
* `d3ea7847f51291291ce49105119095812956bfcf` — chore: pre-termination backup
* `dcfa16ff109e64a206d06321f35d3e3c74b89a97` — chore: pre-termination backup
* `e9b72377d10d3f0e66a7e2c557823418641486ea` — ci: pin Go via go.mod, run Rust tests, add router coverage gate
* `ec3a80ce19e0df630a378904e85406b87825f70a` — chore: add MIT LICENSE, SECURITY.md, and CODEOWNERS

Consequence for any future rewrite: re-minting a message changes the SHA and the GitHub signature; the 15 web-flow `verified` states are lost, the 17 stay unsigned. The **owner** must accept that before Option 2 of `github-attribution-owner-decision.md`.

---

## 8. Local repository state — H (worktrees, branches, stash)

### 8.1 Layout (one repository, two worktrees, one object DB)

* **Main worktree:** `F:/Sehat-Hub` — `.git` directory is `F:/Sehat-Hub/.git`; checked out **`local/r2-drill` @ `704633c`** (== `origin/design/ui-ux-overhaul`).
* **Linked worktree:** `F:/sigap-design-v2` — `.git` is a file: `gitdir: F:/Sehat-Hub/.git/worktrees/sigap-design-v2`; checked out **`feat/sigap-selfhosted-auth` @ `a5423ec`** (104 commits ahead of `origin/main`).
* Refs/objects are shared; only HEAD/index are per-worktree. Any rewrite or `gc` must be run in the shared repo (`F:/Sehat-Hub/.git`) with both worktrees frozen, or both checkouts re-made afterwards.

### 8.2 The 52 local refs (verbatim `git for-each-ref`, `F:/sigap-design-v2`)

```text
f614805cfddc5e23239d81c10c515106da7439d0 refs/heads/ci/bucket-lock-drill
1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3 refs/heads/ci/gate-a-verified
05e19616401003ae669aa0e6870b690489e60de1 refs/heads/ci/lock-drill-refusal-reason
092b1ed86486d15d88472d93deb5de25fb348948 refs/heads/ci/ops-suite-baseline
98470f020df843990d2f29d2b728d8511c6e90b2 refs/heads/ci/r2-list-objects-fix
c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e refs/heads/ci/remove-gitleaks-allowlist
d7404374734a3783224656e37a9714b22c3385af refs/heads/ci/sigap-88-validation
73955d694ec7cd1447db169236aa94c90e536327 refs/heads/ci/sigap-r2-activation
4e470a5332b6db037ed68b2e13873f4a1ff31e4a refs/heads/design/sigap-v2-prototype
d7af334d381a48fbd1054060e5eaffcc6c4521a5 refs/heads/design/ui-ux-overhaul
522fbc48550d357f1ab162a6b316fd4e91c611b7 refs/heads/feat/sigap-regional-pilot
a5423ec35b197395d4d506269d6dd26a1b56b17f refs/heads/feat/sigap-selfhosted-auth
e163189490aeea71ef798ce610a2a455d313de8d refs/heads/feat/sigap-v2-implementation
919c8a797b01ab0890e781f3d0977adabd370e82 refs/heads/fix/auth-form-actions
072b1b1a082a040a3f72fbc328e37671bf5c58d2 refs/heads/fix/grpc-readyz-idle-connect
fb5204a712e08b1703759f4ffa80fdeb81c569f9 refs/heads/fix/prod-localhost-binds
795aec95e03978bb25225ffa7fc9d193814d92e5 refs/heads/fix/request-id-propagation
c86e215322f9ee9f4eabe64b3a59860d2614697d refs/heads/fix/web-supabase-ws-transport
704633cc7f317b9fc3cbe1189bd41f235f4d6d57 refs/heads/local/r2-drill
87f218efe71deca217fbc6c46668e3b0be499503 refs/heads/main
9ec6c52cbe439df8a3295975cf3475e727082dac refs/heads/ops/release-readiness-hardening
3f0f7689d8687d067dc18bd15a6a534317dfe1d8 refs/heads/safety/pre-main-sync-3f0f768
f614805cfddc5e23239d81c10c515106da7439d0 refs/remotes/origin/ci/bucket-lock-drill
1ba464bf9b3e4d7785d22ca1a356b51f9ea623a3 refs/remotes/origin/ci/gate-a-verified
05e19616401003ae669aa0e6870b690489e60de1 refs/remotes/origin/ci/lock-drill-refusal-reason
092b1ed86486d15d88472d93deb5de25fb348948 refs/remotes/origin/ci/ops-suite-baseline
98470f020df843990d2f29d2b728d8511c6e90b2 refs/remotes/origin/ci/r2-list-objects-fix
c443ce0237a0f1645ab0220e0c16e4eb0dd3e93e refs/remotes/origin/ci/remove-gitleaks-allowlist
d7404374734a3783224656e37a9714b22c3385af refs/remotes/origin/ci/sigap-88-validation
73955d694ec7cd1447db169236aa94c90e536327 refs/remotes/origin/ci/sigap-r2-activation
704633cc7f317b9fc3cbe1189bd41f235f4d6d57 refs/remotes/origin/design/ui-ux-overhaul
f449d46033287f613d0d391d741bb3fec3673c64 refs/remotes/origin/docs/audit-701-production-proof
f8b4ef852597db742358a954fc67a85a3d0d88a7 refs/remotes/origin/feat/prod-api-port-override
fbd60b24207ab37f81f4ac626ba8cf8a5e3d93a6 refs/remotes/origin/feat/web-supabase-auth
919c8a797b01ab0890e781f3d0977adabd370e82 refs/remotes/origin/fix/auth-form-actions
68852794f263ec89068bbb4580e090f392afc975 refs/remotes/origin/fix/backup-postgres-pg16-client
6d90116c249657cd33b5cf02e65248d14a43d879 refs/remotes/origin/fix/backup-postgres-server-version-gate
b7aff840981e6521ff9ccaaa98d01265727a156b refs/remotes/origin/fix/compose-auth-env-passthrough
072b1b1a082a040a3f72fbc328e37671bf5c58d2 refs/remotes/origin/fix/grpc-readyz-idle-connect
fb5204a712e08b1703759f4ffa80fdeb81c569f9 refs/remotes/origin/fix/prod-localhost-binds
2595bc327059bf83c34ff915d49cf385a075d68f refs/remotes/origin/fix/prod-ports-override-replace
795aec95e03978bb25225ffa7fc9d193814d92e5 refs/remotes/origin/fix/request-id-propagation
c86e215322f9ee9f4eabe64b3a59860d2614697d refs/remotes/origin/fix/web-supabase-ws-transport
b96c5174ff1cd8e05012e7c27f82ba5a258b3bf3 refs/remotes/origin/main
9ec6c52cbe439df8a3295975cf3475e727082dac refs/remotes/origin/ops/release-readiness-hardening
7419094c3fe4ce1b8b50270530f2fa4a9eea66f6 refs/stash
62754ed83d97e5d887ac92a7893b95f8f5a95851 refs/tags/demo-handoff-green-2026-07-10
001057de41fd4678983da243caff791f1a8cfc71 refs/tags/frontend-demo-green-2026-07-10
fc0df5c8778d4775336b8bd3f13060e21814bff0 refs/tags/local-demo-green-2026-07-09
3380ec26b303e4f1525185a32693b5284c7acb37 refs/tags/v1.0.0-beta
0f454996dd5c7974009e721d221c805942569866 refs/trae_merge/worktree/feat-demo-cleanup-stabilization-1ZK8ui/f4f7c9add59c93e79b195fd37373707bfe6a0e28-chore/demo-data-cleanup/0f454996dd5c7974009e721d221c805942569866/mainHash
f4f7c9add59c93e79b195fd37373707bfe6a0e28 refs/trae_merge/worktree/feat-demo-cleanup-stabilization-1ZK8ui/f4f7c9add59c93e79b195fd37373707bfe6a0e28-chore/demo-data-cleanup/0f454996dd5c7974009e721d221c805942569866/worktreeHash
```

### 8.3 Branches with position vs `origin/main` (`behind/ahead`; last column = commits not on ANY remote ref)

| branch | tip | upstream | behind | ahead | not-on-remote |
|---|---|---|---|---|---|
| `ci/bucket-lock-drill` | `f614805cfddc` | origin/ci/bucket-lock-drill | 4 | 2 | 0 |
| `ci/gate-a-verified` | `1ba464bf9b3e` | origin/ci/gate-a-verified | 5 | 2 | 0 |
| `ci/lock-drill-refusal-reason` | `05e196164010` | origin/ci/lock-drill-refusal-reason | 6 | 2 | 0 |
| `ci/ops-suite-baseline` | `092b1ed86486` | origin/ci/ops-suite-baseline | 9 | 6 | 0 |
| `ci/r2-list-objects-fix` | `98470f020df8` | origin/ci/r2-list-objects-fix | 3 | 1 | 0 |
| `ci/remove-gitleaks-allowlist` | `c443ce0237a0` | origin/ci/remove-gitleaks-allowlist | 7 | 1 | 0 |
| `ci/sigap-88-validation` | `d7404374734a` | origin/ci/sigap-88-validation | 13 | 0 | 0 |
| `ci/sigap-r2-activation` | `73955d694ec7` | origin/ci/sigap-r2-activation | 8 | 21 | 0 |
| `design/sigap-v2-prototype` | `4e470a5332b6` | - | 1 | 18 | 18 |
| `design/ui-ux-overhaul` | `d7af334d381a` | origin/design/ui-ux-overhaul | 5 | 0 | 0 |
| `feat/sigap-regional-pilot` | `522fbc48550d` | - | 0 | 72 | 72 |
| `feat/sigap-selfhosted-auth` | `a5423ec35b19` | - | 0 | 104 | 104 |
| `feat/sigap-v2-implementation` | `e163189490ae` | - | 0 | 49 | 49 |
| `fix/auth-form-actions` | `919c8a797b01` | origin/fix/auth-form-actions | 169 | 1 | 0 |
| `fix/grpc-readyz-idle-connect` | `072b1b1a082a` | origin/fix/grpc-readyz-idle-connect | 171 | 1 | 0 |
| `fix/prod-localhost-binds` | `fb5204a712e0` | origin/fix/prod-localhost-binds | 170 | 1 | 0 |
| `fix/request-id-propagation` | `795aec95e039` | origin/fix/request-id-propagation | 188 | 1 | 0 |
| `fix/web-supabase-ws-transport` | `c86e215322f9` | origin/fix/web-supabase-ws-transport | 172 | 1 | 0 |
| `local/r2-drill` | `704633cc7f31` | origin/design/ui-ux-overhaul | 1 | 0 | 0 |
| `main` | `87f218efe71d` | origin/main | 162 | 0 | 0 |
| `ops/release-readiness-hardening` | `9ec6c52cbe43` | origin/ops/release-readiness-hardening | 2 | 1 | 0 |
| `safety/pre-main-sync-3f0f768` | `3f0f7689d868` | - | 73 | 0 | 0 |

Stash: `stash@{0}` = `7419094c3fe4ce1b8b50270530f2fa4a9eea66f6` ("On main: audit dev fallback request_id - keep docs PR clean") — **2 commits not on any remote ref**. `refs/trae_merge/*`: 2 refs (`0f454996…` mainHash, `f4f7c9ad…` worktreeHash) — tooling bookkeeping, also in the bundle.

---

## 9. Unpublished work — I (what exists only locally)

`git rev-list --all --not --remotes=origin --count` = **107 commits** (any local ref vs all `origin/*`):

| ref | unpublished commits | what it is |
|---|---|---|
| `feat/sigap-selfhosted-auth` (`a5423ec`) | **104** | the active self-hosted-auth line (better-auth, hardening, capability matrix) — nothing of it is on the remote |
| `feat/sigap-regional-pilot` (`522fbc48`) | 72 | regional pilot work; local-only branch |
| `feat/sigap-v2-implementation` (`e1631894`) | 49 | v2 implementation line; local-only |
| `design/sigap-v2-prototype` (`4e470a53`) | 18 | prototype branch; local-only |
| `refs/stash` (`7419094c`) | 2 | stashed work |

The four branch counts overlap (they share ancestors beyond the remote); the distinct total is 107. 6 branch names exist only locally (`design/sigap-v2-prototype`, `feat/sigap-regional-pilot`, `feat/sigap-selfhosted-auth`, `feat/sigap-v2-implementation`, `local/r2-drill`, `safety/pre-main-sync-3f0f768`); 7 remote branches have no local head (`docs/audit-701-production-proof`, `feat/prod-api-port-override`, `feat/web-supabase-auth`, `fix/backup-postgres-pg16-client`, `fix/backup-postgres-server-version-gate`, `fix/compose-auth-env-passthrough`, `fix/prod-ports-override-replace`). **All of it is inside the bundle** (§ 3–4) — that is the point of the drill: the bundle alone can rebuild a repo containing every one of these refs.

---

## 10. Reintroduction risks (what can bring the old attribution/history back)

1. **The backup itself.** The mirror and bundle contain the pre-rewrite history by design. Never add them as a remote of the live repo, never clone from them for daily work, and keep them out of any fetch path. Restoring from them is an explicit, deliberate action only.
2. **A second worktree with shared refs.** `F:/Sehat-Hub` and `F:/sigap-design-v2` share one object DB; a stale branch/index in either worktree can re-push old objects. Freeze or remove both before any remote rewrite.
3. **GitHub keeps unreachable objects.** A force-push does not delete the old commits server-side; any clone of any age, or GitHub Support, can still reach them. Treat the old SHAs as retained until GitHub GC runs, which is not observable from here.
4. **`refs/pull/*` retention.** 93 PR refs keep pre-squash heads fetchable; `git fetch origin refs/pull/*:refs/pull/*` into an active clone re-materialises old objects (not old `main`, but the same objects graph). Do not configure such a refspec.
5. **Stale clones / reflogs / CI caches.** Local reflogs keep old SHAs addressable (~90 days by default); CI jobs pinned to old SHAs, deployment records, image tags and documentation links can re-deploy or re-push them. These were NOT inventoried here (out of scope) — audit them before any cutover.
6. **The stash and `refs/trae_merge/*`.** They live in the shared repo and were snapshotted; pushing them (e.g. `git push --mirror`) would publish old-shaped refs. Push heads/tags explicitly, never `--mirror`, after a rewrite.
7. **Divergent `main`s.** Local `main` = `87f218e` (180 commits, 162 behind `origin/main`) while `origin/main` = `b96c5174` (342 commits). A careless merge/push of the local `main` re-publishes an older line; decide which `main` is authoritative before touching the remote.
8. **GitHub caching windows.** Contributor stats/panels are cached; the docs state up to ~24 h to refresh after history changes. Reading "still shows `claude`" minutes after a rewrite is not evidence of failure — and re-running an "undo" at that point would reintroduce the very thing that was cleaned.
9. **New attribution, same mechanism.** Nothing prevents a future commit from carrying a resolving `Co-authored-by`; without the prevention layer (commit-msg hook + CI check + agent rule from `github-attribution-owner-decision.md` Option 1), the graph can be repopulated by any tool that injects trailers.
10. **Snapshot staleness.** The bundle/mirror freeze the repo at 2026-11-11 13:10. The worktrees are live (other agents commit). **Do not delete any live repo or force-push before a fresh capture**, then re-verify `origin/main` against § 1.

---

## 11. Restore procedure (from this baseline)

```bash
# 1) Everyday restore: a working checkout incl. the unpublished branch
git clone "$env:TEMP\sigap-attribution-backup\sigap-local-all.bundle" sigap-restore

# 2) Exact restore: every ref of the local universe (branches, tags, stash, trae_merge, remote-tracking)
git init --bare sigap-restore.git
git -C sigap-restore.git fetch "$env:TEMP\sigap-attribution-backup\sigap-local-all.bundle" "+refs/*:refs/*"
git -C sigap-restore.git symbolic-ref HEAD refs/heads/feat/sigap-selfhosted-auth
git -C sigap-restore.git fsck --full      # expect exit 0

# 3) Verify against the manifest (every line must match)
#    compare `git -C sigap-restore.git for-each-ref` with refs-manifest-2026-11-11.txt (sections C/D)

# 4) Remote-side rollback (owner approval + force-push credentials REQUIRED; heads+tags only —
#    refs/pull/* cannot be written to GitHub and must not be pushed)
# git -C sigap-mirror.git push --force <remote> refs/heads/*:refs/heads/* refs/tags/*:refs/tags/*
```

---

## 12. Artifacts, hashes, and what this baseline does NOT cover

| artifact | size | sha256 |
|---|---|---|
| `sigap-local-all.bundle` | 19961112 B | `eff7eaf5a523df80cbd2f2db4081000eaa0dd156b3da1ddb0ab8218a478274d9` |
| `sigap-mirror.git/objects/pack/pack-60b7646838d365ab6739733d505ccb42a60d95bb.pack` | 2654156 B | `fb46b1fb465c7ab3b6ade7a85974f644fb2379cc8bbc126da0363b25d3f5826e` |
| `sigap-mirror.git` (whole dir) | 2812734 B | — (pack hash above; `git fsck --full` clean) |
| `refs-manifest-2026-11-11.txt` | 26485 B | `7c974f5dbbd2d812da7985e32c5f876ee311e6f39ec8a2814b11da6d7f1bd237` |
| `evidence/` (all captures) | 7714387 B | per-file hashes in `SHA256SUMS.txt` |

Not covered by this baseline: GitHub-side server objects/reflogs (not observable); Actions caches/artifacts; deployment targets; other clones on other machines; secrets/credentials. These are named in § 10 where they intersect with reintroduction.

---

## 13. Sources

* Live remote: `git ls-remote origin` 2026-11-11 13:40 +07:00 (125 lines) — `evidence/refs-live-2026-11-11.txt`.
* Mirror: `sigap-mirror.git` (created 13:10; refs re-verified vs live 13:40).
* Bundle: `sigap-local-all.bundle` (created 13:10; `git bundle verify` re-run 13:41).
* Restore drill: bare + clone runs 13:42–13:44; outputs in `evidence/drill-bare-refs.txt` and this document.
* API: `api-pulls-stateall-live.json`, `api-tags-live.json`, `api-commit-verification.json` (+ live re-samples), `api-ratelimit-live.json` (50/60 remaining at end of run).

