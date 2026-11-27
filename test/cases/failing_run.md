# Failing run

Used by run.sh to check that unreachable breakpoints are reported even when the run fails.

```bash {"stage":"deploy"}
echo deploy
```

```bash {"stage":"boom"}
false
```
