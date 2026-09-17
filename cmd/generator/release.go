// package: generator / cmd
// type:    logic
// job:     the release-process scenario — a fixed script, not a grown shape
// limits:  builds claims only; delivering them is the client's (-> client.go)
//
// The graph of `docs/use-case-release-process.png`, built for real — a fixed script, where
// `chain` grows an archive by rule. Building it is also a claim about the model: a picture
// that cannot be built cannot honestly be presented. Two things the slide elides: every
// entity is introduced by a source it derives from (D1), and the process runs several times.
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/rankegraph/ranke-go"
)

// The schedule a release actually keeps: minutes to build and test, then days quiet before
// the release window, the scan landing hours before triage rather than at build time.
const (
	buildDelayMin = 4 * time.Minute
	buildDelayMax = 15 * time.Minute

	testDelayMin = 20 * time.Minute
	testDelayMax = 3 * time.Hour

	// The quiet stretch after code is ready, before the release window it waits for.
	devToTriageMin = 2 * 24 * time.Hour
	devToTriageMax = 5 * 24 * time.Hour

	scanLeadMin = 2 * time.Hour
	scanLeadMax = 6 * time.Hour

	candidateDelayMin = 10 * time.Minute
	candidateDelayMax = 40 * time.Minute

	reviewDelayMin = 15 * time.Minute
	reviewDelayMax = 2 * time.Hour

	// Both packages triage the same window, not the same instant.
	packageStaggerMin = 30 * time.Minute
	packageStaggerMax = 90 * time.Minute

	releaseDelayMin = 30 * time.Minute
	releaseDelayMax = 4 * time.Hour

	// Clear of devToTriageMax, so the first release's dev work can't predate setup.
	firstReleaseDelayMin = 7 * 24 * time.Hour
	firstReleaseDelayMax = 10 * 24 * time.Hour

	releaseCadenceMin = 14 * 24 * time.Hour
	releaseCadenceMax = 28 * 24 * time.Hour
)

// jitter draws a duration from [lo, hi) off the grower's own seed, so the schedule looks
// handmade rather than metronomic while staying reproducible (TestReleaseIsDeterministic).
func (g *grower) jitter(lo, hi time.Duration) time.Duration {
	if hi <= lo {
		return lo
	}
	return lo + time.Duration(g.rnd.Int64N(int64(hi-lo)))
}

// atLeast keeps a computed time from drifting earlier than a claim it must follow — a
// claim can never predate what it cites (V-MONO).
func atLeast(t, floor time.Time) time.Time {
	if t.Before(floor) {
		return floor
	}
	return t
}

// laterTime is the later of two, zero losing to anything — the seed value for scanning a
// package's claims for the last one to land.
func laterTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// The scenario's vocabulary. Only `contribution/*` subtypes are reserved (`V-TYPE`), so these
// name what each claim *is*: a reader filtering `derivation/triage` wants triage decisions.
const (
	typeStaffRecord   = "source/staff_record"
	typeRunnerRecord  = "source/runner_registration"
	typePerson        = "entity/person"
	typeCIInstance    = "entity/ci_instance"
	typeGitSnapshot   = "source/git_snapshot"
	typeBuildLog      = "source/build_log"
	typeTestReport    = "source/test_report"
	typeAdvisory      = "source/advisory"
	typeVulnerability = "entity/vulnerability"
	typeScan          = "derivation/vulnerability_scan"
	typeCandidate     = "derivation/release_candidate"
	typeTriage        = "derivation/triage"
	typeReviewNote    = "derivation/review_note"
	typeRelease       = "derivation/release"
)

// The four participants. Each signs what it would sign in life, which is what makes several
// identities worth having: whose claim a thing is says how far to trust it.
const (
	whoRelease  = "release-manager"
	whoSecurity = "security-expert"
	whoTests    = "test-executor"
	whoCI       = "ci-runner"
)

// packages are the two that go out together. Two in one release, rather than one release
// twice, is the point of the picture: the release is where they meet.
var packages = []string{"ranke-go", "ranke-db"}

// vulnerabilities are the advisories the scans matched: one reached by both packages' scans,
// one by a single package's. A claim two provenance paths arrive at is the graph's most
// interesting property. A CVE is public and belongs to no project, so it is one claim.
var vulnerabilities = []struct {
	id, summary string
	// affects names the packages whose scan cites this CVE; empty means all of them.
	affects []string
}{
	{
		id:      "CVE-2024-31337",
		summary: "unbounded allocation in a length-prefixed decoder",
	},
	{
		id:      "CVE-2024-40001",
		summary: "signature check skipped when the algorithm field is absent",
		affects: []string{"ranke-go"},
	},
}

// actor is one participant: the entity the archive holds claims about, and the key it signs
// with. Two claims, because a contributor and an entity never share a node.
type actor struct {
	entity made
	signs  *signer
}

// release runs the process `releases` times, each carrying both packages from snapshot to
// signed-off release. One branch, a release process being one line.
func (g *grower) release(ctx context.Context, branch string, releases int) (batches, error) {
	if releases < 1 {
		return nil, fmt.Errorf("release: --releases must be at least 1")
	}
	// The setup contribution, then per release: seven events per package, plus the release.
	out := make(batches, 0, 1+releases*(len(packages)*7+1))

	// Everything the process refers to, before anything cites it. The root claim rides in
	// ahead of it, added by the caller, since all of this is attributed to the root.
	actors, setup, err := g.actors(ctx, branch)
	if err != nil {
		return nil, err
	}
	found, advisories, err := g.vulnerabilities(branch, actors[whoSecurity])
	if err != nil {
		return nil, err
	}
	out = append(out, batch{branch: branch, claims: append(setup, advisories...)})

	// floor is the earliest a release's own claims may land: every one of them is signed
	// under a key setup minted, so none may predate setup without breaking V-MONO.
	floor := g.at
	// cursor tracks the last thing that happened, so each release's window is scheduled off
	// the one before it — weeks apart, not the seconds a flat clockStep would give it.
	cursor := floor
	for i := range releases {
		gap := g.jitter(releaseCadenceMin, releaseCadenceMax)
		if i == 0 {
			gap = g.jitter(firstReleaseDelayMin, firstReleaseDelayMax)
		}
		releaseDay := cursor.Add(gap)

		version := fmt.Sprintf("v0.%d.0", i+1)
		var triaged []made
		var latestReview time.Time
		for pi, pkg := range packages {
			// Both packages triage in the same window, staggered rather than simultaneous.
			triageAt := releaseDay
			if pi > 0 {
				triageAt = releaseDay.Add(g.jitter(packageStaggerMin, packageStaggerMax))
			}
			bs, decision, reviewedAt, err := g.releaseOnePackage(branch, pkg, version, actors, found, triageAt, floor)
			if err != nil {
				return nil, err
			}
			out = append(out, bs...)
			triaged = append(triaged, decision)
			latestReview = laterTime(latestReview, reviewedAt)
		}

		// The fan-in: one release citing both triage decisions, cut once both packages are
		// truly done — reviewed, not merely decided. A reader following either package
		// upward arrives here, which is what a release *is*.
		artifactAt := latestReview.Add(g.jitter(releaseDelayMin, releaseDelayMax))
		artifact, err := g.write(spec{
			by:      actors[whoRelease].signs,
			branch:  branch,
			typ:     typeRelease,
			fields:  map[string]string{"name": version},
			content: []byte(releaseNotes(version, packages)),
			cites:   asInputs(triaged...),
			at:      artifactAt,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, batch{branch: branch, claims: []ranke.Claim{artifact.claim}})
		cursor = artifactAt
	}
	return out, nil
}

// participants pairs each actor with the type of entity it is, the record introducing it, who
// it is, and the job it held then. Name and role are separate because they are separate
// things: a person is a person, and what they were doing at the time is what a record says
// about them — so the entity carries the name and the record carries the role.
var participants = []struct{ who, entity, record, name, role string }{
	{whoRelease, typePerson, typeStaffRecord, "Ada Okonjo", "release manager"},
	{whoSecurity, typePerson, typeStaffRecord, "Bruno Sallé", "security expert"},
	{whoTests, typePerson, typeStaffRecord, "Chen Wei", "test executor"},
	{whoCI, typeCIInstance, typeRunnerRecord, "ci-runner-07", "x86_64 build runner"},
}

// actors builds the four participants: the introducing record, the entity, and the key the
// root attests — three claims each, in that order, since each rests on the one before.
func (g *grower) actors(ctx context.Context, branch string) (map[string]*actor, []ranke.Claim, error) {
	out := make(map[string]*actor, len(participants))
	claims := make([]ranke.Claim, 0, len(participants)*3)
	for _, p := range participants {
		// An entity needs a path back to a source (D1), and a person or a machine really is
		// introduced by a record. The slide elides these; the seed is richer than the slide.
		record, err := g.write(spec{
			by: g.rootSigner(), branch: branch, typ: p.record,
			fields:  map[string]string{"name": p.who},
			content: []byte(recordText(p.name, p.role, p.who)),
		})
		if err != nil {
			return nil, nil, err
		}
		entity, err := g.write(spec{
			by: g.rootSigner(), branch: branch, typ: p.entity,
			fields:  map[string]string{"name": p.who},
			content: []byte(p.name),
			cites:   asInputs(record),
		})
		if err != nil {
			return nil, nil, err
		}
		signs, err := g.attest(ctx, p.who, entity)
		if err != nil {
			return nil, nil, err
		}
		out[p.who] = &actor{entity: entity, signs: signs}
		claims = append(claims, record.claim, entity.claim, signs.claim)
	}
	return out, claims, nil
}

// vulnerabilities writes each advisory as it arrived and the entity a scan reaches: the
// advisory is what we were told, the entity what an archive links to.
func (g *grower) vulnerabilities(branch string, by *actor) ([]made, []ranke.Claim, error) {
	found := make([]made, 0, len(vulnerabilities))
	claims := make([]ranke.Claim, 0, len(vulnerabilities)*2)
	for _, v := range vulnerabilities {
		advisory, err := g.write(spec{
			by: by.signs, branch: branch, typ: typeAdvisory,
			fields:  map[string]string{"name": v.id},
			content: []byte(logText(v.id+" — "+v.summary, 2)),
		})
		if err != nil {
			return nil, nil, err
		}
		entity, err := g.write(spec{
			by: by.signs, branch: branch, typ: typeVulnerability,
			fields:  map[string]string{"name": v.id},
			content: []byte(v.id + ": " + v.summary),
			cites:   asInputs(advisory),
		})
		if err != nil {
			return nil, nil, err
		}
		found = append(found, entity)
		claims = append(claims, advisory.claim, entity.claim)
	}
	return found, claims, nil
}

// affects reports whether this package's scan reaches CVE number i.
func affects(i int, pkg string) bool {
	return len(vulnerabilities[i].affects) == 0 || slices.Contains(vulnerabilities[i].affects, pkg)
}

// releaseOnePackage takes one package from snapshot to a reviewed decision, a contribution
// per step so the branch table records the process advancing, scheduled around the shared
// triageAt. Returns the decision and when its review note landed, so the caller knows when
// this package was truly done.
func (g *grower) releaseOnePackage(
	branch, pkg, version string,
	actors map[string]*actor,
	found []made,
	triageAt time.Time,
	floor time.Time,
) (batches, made, time.Time, error) {
	name := map[string]string{"name": pkg, "version": version}

	// What arrives from outside. Sources rest on nothing, which is what makes them sources.
	// The dev work — commit, build, test — is done days before the release window it waits
	// for, but never before floor: every claim here is signed under a key setup minted.
	snapshotAt := atLeast(triageAt.Add(-g.jitter(devToTriageMin, devToTriageMax)), floor)
	snapshot, err := g.write(spec{
		by: actors[whoCI].signs, branch: branch, typ: typeGitSnapshot, fields: name,
		content: []byte(snapshotText(pkg, version)), at: snapshotAt,
	})
	if err != nil {
		return nil, made{}, time.Time{}, err
	}
	buildAt := snapshotAt.Add(g.jitter(buildDelayMin, buildDelayMax))
	buildLog, err := g.write(spec{
		by: actors[whoCI].signs, branch: branch, typ: typeBuildLog, fields: name,
		content: []byte(logText(fmt.Sprintf("build %s %s", pkg, version), 6)), at: buildAt,
	})
	if err != nil {
		return nil, made{}, time.Time{}, err
	}
	testAt := buildAt.Add(g.jitter(testDelayMin, testDelayMax))
	testReport, err := g.write(spec{
		by: actors[whoTests].signs, branch: branch, typ: typeTestReport, fields: name,
		content: []byte(logText(fmt.Sprintf("test %s %s", pkg, version), 8)), at: testAt,
	})
	if err != nil {
		return nil, made{}, time.Time{}, err
	}

	// The scan derives from the snapshot and *reaches* the CVEs it matched — not inputs, a
	// scan not being derived from what it reports. It runs close to the decision, not at
	// build time: a fresh read on a candidate about to ship, dated no earlier than the tests
	// it follows in the archive even when the schedule's margin is thin.
	scanAt := atLeast(triageAt.Add(-g.jitter(scanLeadMin, scanLeadMax)), testAt)
	cites := asInputs(snapshot)
	var matched []string
	for i, v := range found {
		if !affects(i, pkg) {
			continue
		}
		cites = append(cites, cite{to: v, typ: edgeMentions, dir: ranke.RelationTo})
		matched = append(matched, vulnerabilities[i].id)
	}
	scan, err := g.write(spec{
		by: actors[whoSecurity].signs, branch: branch, typ: typeScan, fields: name,
		content: []byte(scanText(pkg, version, matched)),
		cites:   cites, at: scanAt,
	})
	if err != nil {
		return nil, made{}, time.Time{}, err
	}

	// The candidate cites all four, which is what makes it a candidate rather than an
	// opinion — assembled once the scan is in, the last of the four to land.
	candidateAt := scanAt.Add(g.jitter(candidateDelayMin, candidateDelayMax))
	candidate, err := g.write(spec{
		by: actors[whoCI].signs, branch: branch, typ: typeCandidate, fields: name,
		content: []byte(fmt.Sprintf("release candidate %s %s", pkg, version)),
		cites:   asInputs(snapshot, buildLog, testReport, scan), at: candidateAt,
	})
	if err != nil {
		return nil, made{}, time.Time{}, err
	}

	// The decision cites the candidate it judges and names who decided, as people rather than
	// as keys. Naming an actor is not provenance. Pinned no earlier than the candidate it
	// judges, the same margin the scan above keeps.
	decisionAt := atLeast(triageAt, candidateAt)
	decided := asInputs(candidate)
	for _, who := range []string{whoRelease, whoSecurity} {
		decided = append(decided, cite{to: actors[who].entity, typ: edgeDecidedBy, dir: ranke.RelationTo})
	}
	decision, err := g.write(spec{
		by: actors[whoRelease].signs, branch: branch, typ: typeTriage, fields: name,
		content: []byte(triageText(pkg, version, matched)),
		cites:   decided, at: decisionAt,
	})
	if err != nil {
		return nil, made{}, time.Time{}, err
	}

	// A remark on the decision, in someone's own words: prose, signed by whoever wrote it,
	// written some time after the decision rather than in the same breath. The slide has no
	// box for this; a real archive is full of them.
	noteAt := decisionAt.Add(g.jitter(reviewDelayMin, reviewDelayMax))
	note, err := g.write(spec{
		by: actors[whoSecurity].signs, branch: branch, typ: typeReviewNote, fields: name,
		content: []byte(reviewNote(pkg, version, matched)),
		cites:   asInputs(decision), at: noteAt,
	})
	if err != nil {
		return nil, made{}, time.Time{}, err
	}

	// One contribution per event, not one covering the whole span: the branch table should
	// advance as the story happens — snapshot, then build, then tests, each its own revision
	// — rather than catch up on all of them at once (V-MONO admits either; realism doesn't).
	return batches{
		{branch: branch, claims: []ranke.Claim{snapshot.claim}},
		{branch: branch, claims: []ranke.Claim{buildLog.claim}},
		{branch: branch, claims: []ranke.Claim{testReport.claim}},
		{branch: branch, claims: []ranke.Claim{scan.claim}},
		{branch: branch, claims: []ranke.Claim{candidate.claim}},
		{branch: branch, claims: []ranke.Claim{decision.claim}},
		{branch: branch, claims: []ranke.Claim{note.claim}},
	}, decision, noteAt, nil
}

// logParagraph is the filler a fake log is padded with. Content this size is the point: it
// makes `output.content.max` decide something, and gives the content routes bytes to fetch.
const logParagraph = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do " +
	"eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, " +
	"quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. " +
	"Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu " +
	"fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa " +
	"qui officia deserunt mollit anim id est laborum."

// logText is a fake log: a title, then filler. Deterministic, so the same seed writes the
// same ids.
func logText(title string, paragraphs int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n\n", title, strings.Repeat("=", len(title)))
	for i := range paragraphs {
		fmt.Fprintf(&b, "[%02d:%02d:%02d] %s\n\n", 9, i*7%60, i*13%60, logParagraph)
	}
	return b.String()
}

// recordText is the record that introduces a participant to the archive. The role is stated
// here and nowhere else: it is a job held when the record was written, which is a claim about
// a person rather than part of who they are — the entity says only the name.
func recordText(name, role, who string) string {
	return fmt.Sprintf("%s\n\nrole: %s\nhandle: %s\nadded to the release process by the archive's root.\n",
		name, role, who)
}

// snapshotText carries enough to recognise the commit; the hash comes from package and
// version, so no two snapshots share one.
func snapshotText(pkg, version string) string {
	sum := sha256.Sum256([]byte(pkg + "@" + version))
	return fmt.Sprintf("%s %s\ncommit %x\nchore(release): prepare %s\n",
		pkg, version, sum[:8], version)
}

// scanText reports what the scan matched.
func scanText(pkg, version string, matched []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "vulnerability scan — %s %s\n\n", pkg, version)
	for _, id := range matched {
		fmt.Fprintf(&b, "  %s  %s\n", id, summaryOf(id))
	}
	fmt.Fprintf(&b, "\n%d advisory match(es), transitive dependencies included\n", len(matched))
	return b.String()
}

// triageText is what was found, and what was decided about it.
func triageText(pkg, version string, matched []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "triage — %s %s\n\n", pkg, version)
	for i, id := range matched {
		verdict := "accepted: not reachable from any exported path"
		if i == 0 {
			verdict = "mitigated: decoder bounded in this release"
		}
		fmt.Fprintf(&b, "  %s  %s\n", id, verdict)
	}
	fmt.Fprint(&b, "\ndecision: ship\n")
	return b.String()
}

// reviewNote is what someone wrote about a decision.
func reviewNote(pkg, version string, matched []string) string {
	if len(matched) > 1 {
		return fmt.Sprintf("Shipping %s %s with %s still open. The decoder path is bounded "+
			"here, so the advisory does not apply to this build; revisit when the shared "+
			"decoder lands upstream.\n\n— security review\n", pkg, version, matched[1])
	}
	return fmt.Sprintf("%s %s carries only the shared advisory, which is mitigated. No "+
		"objection from security.\n\n— security review\n", pkg, version)
}

// summaryOf is a CVE's one-line summary, by id.
func summaryOf(id string) string {
	for _, v := range vulnerabilities {
		if v.id == id {
			return v.summary
		}
	}
	return ""
}

// releaseNotes is what the release itself says, naming what went out together.
func releaseNotes(version string, of []string) string {
	return fmt.Sprintf("release %s\n\npackages: %s\ntriage: both packages signed off\n",
		version, strings.Join(of, ", "))
}
