"""Offline checks of the bot's pure logic: python3 .bot/test_bot.py (exit 1 on failure)."""

import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
# installed: .bot/project.toml next to bot.py; in the bots' own repository: the pilot's example
if not os.path.exists(os.path.join(HERE, "project.toml")):
    os.environ.setdefault("BOT_PROJECT", os.path.join(HERE, "..", "examples", "markdown-runner", "project.toml"))
sys.path.insert(0, HERE)
import bot  # noqa: E402

FAILED = 0


def check(name, cond):
    global FAILED
    print(("ok   " if cond else "FAIL ") + name)
    FAILED += not cond


def pr(**kw):
    f = {
        "number": 5, "sha": "abc", "branch": "bot/issue-1", "issue": 1, "labels": set(), "ci": "success",
        "failing": [], "failed_runs": [], "alerts": [], "review": None, "human": [], "review_rounds": 0,
        "fix_rounds": 0, "human_rounds": 0, "fix_history": [],
    }
    f.update(kw)
    return f


k = lambda f: (bot.decide_pr(f) or {}).get("kind")  # noqa: E731
check("no CI yet -> dispatch", k(pr(ci="none")) == "dispatch_ci")
check("CI running -> wait", k(pr(ci="pending")) == "wait_ci")
check("CI red -> fix", k(pr(ci="failure", failing=["test"])) == "fix")
check("same failure after two fixes -> stuck",
      k(pr(ci="failure", failing=["test"], fix_history=[["test"], ["test"]], fix_rounds=2)) == "stuck")
check("fix budget -> stuck", k(pr(ci="failure", failing=["a"], fix_rounds=bot.FIX_ROUNDS)) == "stuck")
check("green, unreviewed -> review", k(pr()) == "review")
check("review budget -> stuck", k(pr(review_rounds=bot.REVIEW_ROUNDS)) == "stuck")
check("changes -> address", k(pr(review="changes")) == "address")
check("ready -> handover", k(pr(review="ready")) == "handover")
check("handed over -> nothing", k(pr(review="ready", labels={bot.L_READY})) is None)
check("human remark beats ready", k(pr(review="ready", labels={bot.L_READY}, human=[{"id": "c1"}])) == "address")
check("stuck -> nothing", k(pr(ci="failure", labels={bot.L_STUCK})) is None)

check("PR cost cap stops a review", k(pr(usd=bot.PR_USD)) == "stuck")
check("PR cost cap stops a fix", k(pr(ci="failure", failing=["a"], usd=bot.PR_USD + 1)) == "stuck")
check("PR cost cap doesn't block the handover", k(pr(review="ready", usd=bot.PR_USD + 1)) == "handover")
check("under the cap -> review", k(pr(usd=bot.PR_USD - 1)) == "review")
mk = lambda kind, usd=0: {"kind": kind, "stats": {"usd": usd}}  # noqa: E731
check("spend counted since the last stuck",
      bot.spent_since_stuck([mk("new", 30), mk("review", 25), mk("stuck"), mk("fix", 4)]) == 4)
b = bot.budgets([mk("review", 1), mk("review", 1), mk("review", 1), mk("stuck"), mk("review", 2)])
check("removing stuck restarts the review budget", b["review_rounds"] == 1 and b["usd"] == 2)
check("spend without stuck", bot.spent_since_stuck([mk("new", 30), mk("review", 25)]) == 55)

iss = lambda n, v=0, labels=(): {"number": n, "title": f"t{n}", "labels": [{"name": x} for x in labels],  # noqa: E731
                                 "reactions": {"+1": v}}
a = bot.decide([], [iss(3, 1), iss(7, 4), iss(2, 1)])
check("most voted first", a["kind"] == "new" and a["issue"] == 7)
a = bot.decide([], [iss(3, 1), iss(2, 1)])
check("tie -> oldest", a["issue"] == 2)
a = bot.decide([pr(ci="pending")], [iss(9)])
check("a PR waiting on CI doesn't block a new issue", a["kind"] == "new" and a["issue"] == 9)
a = bot.decide([pr(ci="pending"), pr(number=6, ci="pending", issue=2)], [iss(9)])
check("max PRs in flight -> wait instead", a["kind"] == "wait_ci")
a = bot.decide([pr(review="changes")], [iss(9, 10)])
check("finish started work before a new issue", a["kind"] == "address")

diff = """diff --git a/runner/runner.go b/runner/runner.go
--- a/runner/runner.go
+++ b/runner/runner.go
@@ -1 +1 @@
-x
+y // TODO later
diff --git a/runner/runner_test.go b/runner/runner_test.go
--- a/runner/runner_test.go
+++ b/runner/runner_test.go
@@ -1 +1 @@
-	assert.Equal(t, 1, f())
+	_ = f()
diff --git a/go.mod b/go.mod
"""
r = bot.report(diff)
check("report: protected go.mod", r["protected_touched"] == ["go.mod"])
check("report: removed assertion", len(r["removed_test_checks"]) == 1)
check("report: debug leftover", len(r["debug_leftovers"]) == 1)
r["without_change"] = {"checked": True, "pass_on_old_code": ["go test"], "fail_on_old_code": []}
hard = bot.hard_findings(r)
check("hard: tests that pass on old code", any("WITHOUT" in h for h in hard))

model = {"verdict": "ready", "summary": "fine", "unhappy": [{"case": "empty", "handled": True, "tested": True}],
         "probes": [{"input": "a", "got": "OK  one", "ok": True}, {"input": "b", "got": "two", "ok": True},
                    {"input": "c", "got": "invented", "ok": True}]}
out = bot.enforce(model, [], ["...OK one...", "two"], code_changed=True)
check("invented probe output -> changes", out["verdict"] == "changes")
check("verified flag", [p["verified"] for p in out["probes"]] == [True, True, False])
model["probes"] = model["probes"][:2] + [{"input": "c", "got": "three", "ok": True}]
out = bot.enforce(model, [], ["OK one", "two", "three"], code_changed=True)
check("all verified, all ok -> ready", out["verdict"] == "ready")
model["unhappy"].append({"case": "nil", "handled": False, "tested": False})
check("unhandled path -> changes", bot.enforce(model, [], ["OK one two three"], True)["verdict"] == "changes")

raw = "\x1b[30;46m\x1b[30;46m INFO \x1b[0m\x1b[0m \x1b[96m/tmp/p/md/demo.md:\x1b[0m\x1b[0m✅"
m2 = {"verdict": "ready", "summary": "ok", "unhappy": [], "probes": [{"input": "x", "got": "INFO /tmp/p/md/demo.md:✅", "ok": True}]}
check("probe output found through colour codes", bot.enforce(m2, [], [raw], code_changed=False)["probes"][0]["verified"])
m3 = {"verdict": "ready", "summary": "ok", "unhappy": [], "probes": [
    {"input": "x", "got": "[30;46m[30;46m INFO [0m[0m [96m[96m/tmp/rv1/d/a.md:[0m[0m✅", "ok": True},
    {"input": "y", "got": "[96m[96mecho other [95m[95m[Yes/no/continue/quit][0m[96m[0m[96m: [0m[0m", "ok": True}]}
raw3 = ["\x1b[30;46m\x1b[30;46m INFO \x1b[0m\x1b[0m \x1b[96m\x1b[96m/tmp/rv1/d/a.md:\x1b[0m\x1b[0m✅",
        "\x1b[96m\x1b[96mecho other \x1b[95m\x1b[95m[Yes/no/continue/quit]\x1b[0m\x1b[96m\x1b[0m\x1b[96m: \x1b[0m\x1b[0m"]
check("probe copied with bare colour codes is verified",
      all(p["verified"] for p in bot.enforce(m3, [], raw3, code_changed=False)["probes"]))
check("[Yes/no] is not a colour code", "[Yes/no/continue/quit]" in bot._norm("[95m[Yes/no/continue/quit][0m"))
# Notes for the human and the coder's pushback
RUNS = ["OK one", "two", "three", "-2"]
GOOD = [{"input": "a", "got": "OK one", "ok": True}, {"input": "b", "got": "two", "ok": True},
        {"input": "c", "got": "three", "ok": True}]


def rv(unhappy, probes=()):
    return bot.enforce({"verdict": "ready", "summary": "s", "unhappy": unhappy, "probes": GOOD + list(probes)},
                       [], RUNS, code_changed=True)


def wrong(case=""):
    return {"input": "-1", "got": "-2", "ok": False, "case": case}


def says(out, words):
    return any(words in c["body"] for c in out["comments"])


OLD = {"case": "Crash on  empty input", "handled": False, "tested": False, "pre_existing": True}
LIM = {"case": "huge files", "handled": False, "tested": False, "limitation": True, "note": "README: < 1 MB"}
out = rv([OLD], [wrong("  crash on empty INPUT ")])
check("pre-existing case and its wrong probe: a note, ready (case copied loosely)",
      out["verdict"] == "ready" and says(out, "Not for this PR (the base branch already has it)"))
out = rv([LIM], [wrong("huge files")])
check("documented limitation and its probe: a note, ready", out["verdict"] == "ready" and says(out, "deliberate limitation"))
check("a pre-existing case handled and tested is no note", not says(rv([{**OLD, "handled": True, "tested": True}]), "Not for this PR"))
check("a wrong probe naming no case still blocks", rv([OLD], [wrong()])["verdict"] == "changes")
check("a wrong probe on a case that is no note still blocks",
      rv([OLD, {"case": "nil", "handled": True, "tested": True}], [wrong("nil")])["verdict"] == "changes")
DIS = {"case": "a 20k-char line", "handled": False, "tested": False, "disputed": True}
out = rv([DIS], [wrong(DIS["case"])])
check("disputed and not answered: a note for the human, its probe too",
      out["verdict"] == "ready" and says(out, "the coder disputed it, the review doesn't answer"))
SAID = {**DIS, "rebuttal": "still wrong, as I said"}
check("repeating the remark is no answer", rv([SAID])["verdict"] == "ready")
out = rv([SAID], [wrong(SAID["case"])])
check("a rebuttal with this round's wrong probe: changes, the answer shown",
      out["verdict"] == "changes" and says(out, "Unhandled unhappy path: a 20k-char line")
      and says(out, "the review answers: still wrong"))
out = rv([SAID], [{**wrong(SAID["case"]), "got": "trust me, it breaks"}])
check("a paraphrased probe is no answer (and voids the review as before)",
      says(out, "the review doesn't answer") and says(out, "Unverifiable review"))
out = rv([SAID], [wrong("another case")])
check("a rebuttal backed by a wrong probe on ANOTHER case is no answer to this one",
      says(out, "the review doesn't answer") and not says(out, "Unhandled unhappy path: a 20k-char line"))
check("a new unhandled case still blocks", rv([{"case": "nil", "handled": False, "tested": False}])["verdict"] == "changes")

prev = bot.previous_round(
    [{"user": {"login": "rev[bot]"}, "state": "CHANGES_REQUESTED", "submitted_at": "2026-11-25T10:00:00Z", "body": "OLD"},
     {"user": {"login": "rev[bot]"}, "state": "DISMISSED", "submitted_at": "2026-11-25T11:00:00Z", "body": "BODY-42"},
     {"user": {"login": "alice"}, "state": "COMMENTED", "submitted_at": "2026-11-25T13:00:00Z", "body": "HUMAN"}],
    [{"user": {"login": "cod[bot]"}, "created_at": "2026-11-25T10:30:00Z", "body": "BEFORE"},
     {"user": {"login": "cod[bot]"}, "created_at": "2026-11-25T12:00:00Z", "body": "I disagree: run 3 <!-- bot {\"kind\": \"address\"} -->"}], reviewer="rev[bot]", coder="cod[bot]")
check("previous round: the reviewer's last review and the coder's replies since, markers stripped",
      "BODY-42" in prev and "I disagree: run 3" in prev and "OLD" not in prev and "BEFORE" not in prev
      and "HUMAN" not in prev and "<!--" not in prev)
check("no previous review, no previous round", bot.previous_round([], [], "rev[bot]", "cod[bot]") == "")
check("the reviewer is told about pushback, the coder that it may push back",
      "`rebuttal`" in bot.REVIEWER_SYSTEM and "push back" in open(bot.__file__).read())

out = bot.enforce({"verdict": "ready", "summary": "s", "unhappy": [], "probes": GOOD[:2]}, [], RUNS, code_changed=True)
check("fewer than 3 verified probes of changed code: incomplete review",
      out["verdict"] == "changes" and says(out, "fewer than 3 verified probes"))
check("…but enough when no code changed",
      bot.enforce({"verdict": "ready", "summary": "s", "unhappy": [], "probes": GOOD[:2]}, [], RUNS, code_changed=False)["verdict"] == "ready")
_rep = {"protected_touched": [], "code_touched": ["runner/x.go"], "tests_touched": [], "without_change": {"checked": False}}
check("code changed without any test: a hard finding", any("no test" in h for h in bot.hard_findings(_rep)))
check("code changed with a test: none", bot.hard_findings({**_rep, "tests_touched": ["runner/x_test.go"]}) == [])

# Failures named per test, not per job
LOG1 = ("2026-11-25T10:00:00.1234567Z === RUN   TestA\n"
        "2026-11-25T10:00:00.2Z \x1b[31m--- FAIL: TestBreakpoint (0.01s)\x1b[0m\n"
        "2026-11-25T10:00:00.3Z     --- FAIL: TestBreakpoint/sub_case (0.00s)\n")
check("failing tests named from a job log (timestamps, colours, durations stripped)",
      bot.scenarios(LOG1) == {"TestBreakpoint", "TestBreakpoint/sub_case"})
check("a log naming no test: the job's name", bot.failure_names("integration", "make: *** Error 1") == {"integration"})
check("…else « job: test »", bot.failure_names("go", LOG1) == {"go: TestBreakpoint", "go: TestBreakpoint/sub_case"})
bot.FAILURE_PATTERNS.append(bot.re.compile(r"^\s+\d+\) (\[[^\]]+\] › .+?)\s*─*$", bot.re.M))
PW = lambda line: f"  1) [chromium] › e2e/login.spec.ts:{line}:5 › login works ───────\n"  # noqa: E731
check("the same e2e test keeps its name when its line moves", bot.scenarios(PW(12)) == bot.scenarios(PW(40))
      == {"[chromium] › e2e/login.spec.ts › login works"})
bot.FAILURE_PATTERNS.pop()
e2e1, e2e2 = ["e2e: [chromium] › a.spec.ts › login"], ["e2e: [chromium] › b.spec.ts › logout"]
check("the same job failing on a different test after two fixes: progress, fix again",
      k(pr(ci="failure", failing=e2e2, fix_history=[e2e1, e2e1], fix_rounds=2)) == "fix")
check("…the same test three times: stuck",
      k(pr(ci="failure", failing=e2e1, fix_history=[e2e1, e2e1], fix_rounds=2)) == "stuck")

# The old-code check says what it proved, or why it didn't run
check("old-code line: tests fail on the old code",
      "✅" in bot.old_code_line({"checked": True, "fail_on_old_code": ["go test"], "pass_on_old_code": []}))
check("old-code line: tests pass on the old code",
      "❌" in bot.old_code_line({"checked": True, "fail_on_old_code": [], "pass_on_old_code": ["go test"]}))
check("old-code line: e2e only, not checkable, said so",
      "not run (the PR's tests run in CI only" in bot.old_code_line(
          {"checked": False, "why": "the PR's tests run in CI only (e2e): not checkable here", "ci_only": ["e2e/a.spec.ts"]})
      and "`e2e/a.spec.ts`" in bot.old_code_line({"checked": False, "why": "x", "ci_only": ["e2e/a.spec.ts"]}))

# Per-check timeouts: a timeout is reported, never a crash, and proves nothing on the old code
import subprocess  # noqa: E402
import tempfile as _tf  # noqa: E402
_h = _tf.mkdtemp()
bot.TIMEOUT_MIN[("gate", "slow")] = 0.01
_nap = f"sleep {30 + os.getpid() % 1000 / 1000:.3f}"  # a command line only this run has
_t0 = bot.time.monotonic()
_ok, _out = bot.run_check("gate", "slow", ["sh", "-c", f"echo started; {_nap} & {_nap}; wait"], _h, _h)
_took = bot.time.monotonic() - _t0
check("a check over its timeout: None, said so, its output kept", _ok is None and "timed out after 0.01 min" in _out
      and "started" in _out)
_left = subprocess.run(["pgrep", "-fx", _nap], capture_output=True, text=True).stdout.split()
check("…and its whole process tree is killed, at once", _left == [] and _took < 10)
check("a passing check", bot.run_check("gate", "fast", ["sh", "-c", "true"], _h, _h)[0] is True)
check("a failing check, with its output", bot.run_check("gate", "fast", ["sh", "-c", "echo boom; false"], _h, _h) == (False, "boom\n"))

s, m = bot.parse_final("blah\nSUBJECT: warn on unreachable breakpoint\nSUMMARY: did it.\nmore")
check("parse_final", s == "warn on unreachable breakpoint" and m.startswith("did it."))
check("legacy and current markers", bot.MARK.search('<!-- mr-bot {"kind": "a"} -->') and bot.MARK.search('<!-- bot {"kind": "b"} -->'))
check("config: tests vs code", bot.is_test("runner/x_test.go") and bot.is_test("test/run.sh") and bot.is_code("runner/x.go") and not bot.is_code("runner/x_test.go"))
check("config: gate from project.toml", [n for n, _ in bot.GATE][:3] == ["gofmt", "go vet", "go test"])
check("config: agent env expands {home}", bot.agent_env("/h")["GOPATH"] == "/h/go")
check("config: allowed commands added", "Bash(go:*)" in bot.CODER_TOOLS and "Bash(go:*)" in bot.REVIEWER_TOOLS)
check("config: prompts name the project", "markdown-runner" in bot.CODER_SYSTEM and "mr-probe" in bot.REVIEWER_SYSTEM)
_ms = [{"kind": "review"}] * 3 + [{"kind": "fix", "failing": ["x"]}] * 2 + [{"kind": "handover"}, {"kind": "address", "source": "human"}]
check("a human review starts a new bot cycle", bot.budgets(_ms)["review_rounds"] == 0 and bot.budgets(_ms)["fix_rounds"] == 0
      and bot.budgets(_ms)["human_rounds"] == 1)
check("reviews still count within a cycle", bot.budgets(_ms + [{"kind": "review"}] * 2)["review_rounds"] == 2)
check("protected glob", bot.protected(".github/workflows/x.yml") and not bot.protected("runner/x.go"))

# ---------------------------------------------------------------- the Codex backend (offline: recorded event shapes)
def _strict_ok(sc):
    if sc.get("type") == "object":
        ok = sc.get("additionalProperties") is False and sorted(sc["required"]) == sorted(sc["properties"])
        return ok and all(_strict_ok(v) for v in sc["properties"].values())
    return _strict_ok(sc["items"]) if sc.get("type") == "array" else True


_ss = bot.strict_schema(bot.REVIEW_SCHEMA)
check("codex: the review schema is strict (closed objects, every key required)", _strict_ok(_ss))
_pe = _ss["properties"]["unhappy"]["items"]["properties"]["pre_existing"]
check("codex: an optional key becomes nullable, a required one doesn't",
      _pe["type"] == ["boolean", "null"] and _ss["properties"]["verdict"]["type"] == "string"
      and None not in _ss["properties"]["verdict"]["enum"])
check("codex: the original schema is untouched", "additionalProperties" not in bot.REVIEW_SCHEMA)
check("codex: nulls are stripped back out",
      bot.strip_nulls({"a": None, "b": [{"c": None, "d": 1}]}) == {"b": [{"d": 1}]})

_j = json.dumps
_verdict = {"verdict": "changes", "summary": "s", "comments": [{"path": "a.go", "line": None, "body": "b"}],
            "unhappy": [], "probes": [{"input": "x", "got": "Add(2,3) = -1", "ok": False, "case": None}]}
_events = [
    _j({"type": "thread.started", "thread_id": "t-1"}),
    "not json: a warning line",
    _j({"type": "item.completed", "item": {"id": "i0", "type": "error", "message": "Model metadata not found"}}),
    _j({"type": "item.started", "item": {"type": "command_execution", "command": "go run .", "aggregated_output": "",
                                         "status": "in_progress"}}),
    _j({"type": "item.completed", "item": {"type": "command_execution", "command": "go run .",
                                           "aggregated_output": "\x1b[31mAdd(2,3) = -1\x1b[0m\n", "exit_code": 0,
                                           "status": "completed"}}),
    _j({"type": "item.completed", "item": {"type": "agent_message", "text": _j(_verdict)}}),
    _j({"type": "turn.completed", "usage": {"input_tokens": 1_000_000, "cached_input_tokens": 600_000,
                                            "output_tokens": 100_000}}),
]
_r = bot.parse_codex(_events, "gpt-6.2-sol", schema=True)
check("codex: a finished turn is a success, with its session", not _r["is_error"] and _r["session_id"] == "t-1")
check("codex: command outputs are kept for the probe check", _r["tool_outputs"] == ["\x1b[31mAdd(2,3) = -1\x1b[0m\n"])
check("codex: the structured verdict, nulls stripped",
      _r["structured_output"]["comments"][0] == {"path": "a.go", "body": "b"} and "case" not in _r["structured_output"]["probes"][0])
check("codex: priced per fresh, cached and output tokens",
      abs(_r["total_cost_usd"] - (0.4 * 2.0 + 0.6 * 0.10 + 0.1 * 10.0)) < 1e-9)
check("codex: usage in the harness's shape", _r["usage"] == {"input_tokens": 400_000, "cache_read_input_tokens": 600_000,
                                                            "output_tokens": 100_000})
_out = bot.enforce(_r["structured_output"], [], _r["tool_outputs"], code_changed=False)
check("codex: a probe quoted from a command's output is verified", _out["probes"][0]["verified"])
_fake = dict(_verdict, probes=[{"input": "x", "got": "Add(2,3) = 5", "ok": True}])
_out = bot.enforce(_fake, [], _r["tool_outputs"], code_changed=False)
check("codex: a probe no command printed is not verified", not _out["probes"][0]["verified"])
_fail = bot.parse_codex([_j({"type": "thread.started", "thread_id": "t"}),
                         _j({"type": "turn.failed", "error": {"message": "HTTP 429: monthly budget exceeded"}})], "gpt-6.2-sol", True)
check("codex: a 429 is an error, named as a rate limit", _fail["is_error"] and _fail["subtype"] == "error_rate_limit")
_bad = bot.parse_codex([_j({"type": "item.completed", "item": {"type": "agent_message", "text": "no JSON here"}}),
                        _j({"type": "turn.completed", "usage": {}})], "gpt-6.2-sol", True)
check("codex: a verdict that isn't JSON is an error, not an empty review", _bad["is_error"])
check("codex: without a schema, the last message is the result",
      bot.parse_codex([_j({"type": "item.completed", "item": {"type": "agent_message", "text": "SUBJECT: x"}}),
                       _j({"type": "turn.completed", "usage": {}})], "gpt-5.3-codex", False)["result"] == "SUBJECT: x")
_resumed = bot.parse_codex([_j({"type": "thread.started", "thread_id": "t-1"}),
                            _j({"type": "item.completed", "item": {"type": "agent_message", "text": "ok"}}),
                            _j({"type": "turn.completed", "usage": {"input_tokens": 1_100_000, "cached_input_tokens": 650_000,
                                                                    "output_tokens": 110_000}})],
                           "gpt-6.2-sol", False, prior=_r["codex_usage"])
check("codex: a resumed round pays only for its own tokens (Codex reports the thread's total)",
      _resumed["usage"] == {"input_tokens": 50_000, "cache_read_input_tokens": 50_000, "output_tokens": 10_000}
      and abs(_resumed["total_cost_usd"] - (0.05 * 2.0 + 0.05 * 0.10 + 0.01 * 10.0)) < 1e-9)
_h = bot.agent_home()
check("every agent home has the TMPDIR a Codex agent and its checks are given", os.path.isdir(os.path.join(_h, "tmp")))
_removed = []
_orig = (bot.remove_label, bot.codex)
bot.remove_label = lambda n, label: _removed.append((n, label))
bot.codex = lambda *a, **kw: {"subtype": "error_rate_limit", "is_error": True, "error": "HTTP 429"}
_saved_agent, bot.AGENT = bot.AGENT, "codex"
_acts = bot.Actions.__new__(bot.Actions)
_acts.dry, _acts.conv = False, ""
_acts.do_new = lambda a, f: bot.agent("/tmp", "p", "s", "l", [], "m")
try:
    _acts.run({"kind": "new", "issue": 9, "title": "t"}, None)
    _raised = False
except bot.RateLimited:
    _raised = True
check("a 429 stops the run and puts a new issue back in the queue", _raised and _removed == [(9, bot.L_DOING)])
_removed.clear()
_acts.do_new = lambda a, f: 1 / 0
try:
    _acts.run({"kind": "new", "issue": 8, "title": "t"}, None)
except ZeroDivisionError:
    pass
check("a crash starting an issue puts it back in the queue too", _removed == [(8, bot.L_DOING)])
_h2 = bot.agent_home()
bot.codex_config(_h2)
os.chmod(os.path.join(_h2, ".codex"), 0o555)  # what the agent user's files look like to the harness
try:
    bot.codex_config(_h2)
    _again = True
except OSError:
    _again = False
os.chmod(os.path.join(_h2, ".codex"), 0o755)
check("codex: a resume doesn't touch the agent's files again", _again)
check("codex: default models are in Codex's catalog, and the reviewer differs",
      _saved_agent != "codex" or (bot.MODEL, bot.REVIEW_MODEL) == ("gpt-6.2-sol", "gpt-6.2-astra"))
bot.codex = lambda *a, **kw: {"subtype": "success", "is_error": False, "result": "fine"}
check("a run that went fine is returned as is", bot.agent("/tmp", "p", "s", "l", [], "m")["result"] == "fine")
bot.remove_label, bot.codex = _orig
bot.AGENT = _saved_agent
check("codex: no turn.completed is an error", bot.parse_codex([_j({"type": "thread.started", "thread_id": "t"})], "m", False)["is_error"])
sys.exit(1 if FAILED else 0)
