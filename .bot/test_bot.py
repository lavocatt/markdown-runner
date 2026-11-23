"""Offline checks of the bot's pure logic: python3 .bot/test_bot.py (exit 1 on failure)."""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
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

s, m = bot.parse_final("blah\nSUBJECT: warn on unreachable breakpoint\nSUMMARY: did it.\nmore")
check("parse_final", s == "warn on unreachable breakpoint" and m.startswith("did it."))
check("protected glob", bot.protected(".github/workflows/x.yml") and not bot.protected("runner/x.go"))
sys.exit(1 if FAILED else 0)
