# v1.0.0 Tag Day Script

One-shot procedure for cutting the v1.0.0 release. Delete this file (`git rm TAG_V1.md`) after the tag is pushed.

The general pre-tag checklist lives in `RELEASE.md`; this file is the v1.0-specific play-by-play so you can return to it cold after the soak window.

## Pre-flight (the day before tag day)

```bash
# 1. Pull latest, confirm clean.
git pull origin main
git status                          # must be clean

# 2. Verify v0.9.1 has soaked at least 2 weeks AND no new issues filed.
gh issue list --state open --limit 20

# 3. Confirm pkg.go.dev rendered v0.9.1 fine (open in browser):
#    https://pkg.go.dev/github.com/Sriram-PR/go-ignore@v0.9.1

# 4. If fuzz-long has not been run in the last week, trigger it now.
#    Open the Actions tab on GitHub, "Run workflow" -> job: fuzz-long.
#    Wait for completion (~5 hours). Must be green.
```

If any of (2), (3), or (4) fail: stop. Fix as a v0.9.x patch first. Do not tag v1.0 over a known issue.

## Tag day — morning

```bash
# 1. Full local verification.
make ci                             # fmt, vet, test, test-race, lint
make fuzz                           # 30s smoke per fuzzer; ~4 min total

# 2. Run examples to confirm pkg.go.dev rendering won't regress.
go test -run "^Example" -v ./...

# 3. Coverage sanity check (should be ~95%+).
go test -cover ./...
```

## Tag day — write the release notes

Open `RELEASE.md`. Add a new section **above** `### v0.9.1`:

```markdown
### v1.0.0

API freeze. Within the 1.x line: no breaking changes to exported types,
functions, methods, fields, or constants. New functionality may be added;
defensive limit constants may be raised but never lowered. See README's
"Stability Guarantees" section for the full contract.

**No code changes from v0.9.1.** This release exists to commit to the
v1.x semver contract. Callers on v0.9.1 should pin to `v1.0.0` and gain
the explicit stability guarantee.
```

Also update the v0.9.1 section: remove the "**Note:** known issue" callout from **v0.9.0** if pkg.go.dev's index has caught up (the v0.9.0 → v0.9.1 upgrade message can stay forever — that's accurate history).

Commit:

```bash
git add RELEASE.md
git commit -m "release: prepare v1.0.0"
```

Also bump auto-memory `MEMORY.md` (in your `~/.claude/projects/...` directory) — change `Current release: v0.9.1` to `Current release: v1.0.0` and add a one-line v1.0.0 entry to the versioning list.

## Tag day — create the tag (DO NOT push yet)

```bash
git tag -a v1.0.0 -m "v1.0.0: API freeze for the v1.x line"
git log -1 --oneline v1.0.0         # confirm it points at the RELEASE commit
```

## Sleep on it

Step away. Overnight if possible. The v0.9.0 → v0.9.1 patch in this project happened because a tag was pushed too quickly. **Don't repeat it.**

## Next morning — re-verify, then push

```bash
make ci                             # nothing should have regressed overnight
git log -1 --oneline v1.0.0         # confirm tag is still where you put it

# Push commits, then tag (in that order; tag push depends on commit being on origin)
git push origin main
git push origin v1.0.0
```

## Within 30 minutes after push

1. **Verify pkg.go.dev:**
   `https://pkg.go.dev/github.com/Sriram-PR/go-ignore@v1.0.0`

   Must show:
   - Version selector dropdown lists v1.0.0
   - All 8 Examples render in the Examples tab
   - README renders in the Overview tab
   - No "failed to fetch" banner

   If pkg.go.dev fails: do **not** panic-fix. Wait up to an hour for indexing. If still broken after 24 hours, file an issue at `golang/pkgsite` or use the "Request" button on pkg.go.dev.

2. **Create the GitHub Release** (`gh release create v1.0.0` or via web UI):
   - Title: `v1.0.0`
   - Body: paste the `### v1.0.0` section from `RELEASE.md`
   - Mark as "Latest release"

3. **Delete this file:**
   ```bash
   git rm TAG_V1.md
   git commit -m "chore: remove v1.0 tag-day script after release"
   git push origin main
   ```

## Post-release (optional, after a few days)

- Submit to [awesome-go](https://github.com/avelino/awesome-go) under the file-handling or git-tools category.
- Consider a brief HN / r/golang post — "go-ignore v1.0: zero-dep, git-parity-tested gitignore matching for Go." Reference the comparison table in the README rather than rehashing it.
- Watch issues for the first week — any v1.0-specific complaint shapes v1.1.

## If something goes catastrophically wrong after pushing v1.0.0

**Do not force-push the tag.** v0.9.0's lesson stands.

If a critical bug ships in v1.0.0:
1. Add a `**Note:** known issue, upgrade to v1.0.1` callout to the v1.0.0 entry in `RELEASE.md`.
2. Patch in main.
3. Tag v1.0.1 following the same procedure as above (without the soak — patches don't need it).

The cost of v1.0.0 being "the buggy first stable" is small. The cost of breaking the immutability contract is forever.
