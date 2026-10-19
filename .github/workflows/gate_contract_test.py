#!/usr/bin/env python3

from __future__ import annotations

import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/conformance.yml"
GATE = ROOT / "script/gate.sh"
FUZZ_SOURCE = ROOT / "opml_fuzz_test.go"
ROOT_GO_MOD = ROOT / "go.mod"
ROOT_GO_SUM = ROOT / "go.sum"
TOOLS_GO_MOD = ROOT / "tools/go.mod"
TOOLS_GO_SUM = ROOT / "tools/go.sum"
SDK_IF = "${{ inputs.sdk_ref != '' }}"
FUZZ_IF = "${{ github.event_name == 'schedule' || github.event_name == 'workflow_dispatch' }}"
SDK_CHECKOUT = """      - name: Checkout SDK override
        if: ${{ inputs.sdk_ref != '' }}
        uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4.3.1
        with:
          repository: beamfall/plugin-sdk
          ref: ${{ inputs.sdk_ref }}
          path: beamfall-plugin-sdk"""
SDK_USE = """      - name: Use SDK override
        if: ${{ inputs.sdk_ref != '' }}
        working-directory: beamfall-plugin-podcasts
        run: go mod edit -replace github.com/beamfall/plugin-sdk=../beamfall-plugin-sdk"""
SDK_STEPS = {
    "Checkout SDK override": {
        "if": SDK_IF,
        "uses": "actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4.3.1",
        "with": {
            "repository": "beamfall/plugin-sdk",
            "ref": "${{ inputs.sdk_ref }}",
            "path": "beamfall-plugin-sdk",
        },
    },
    "Use SDK override": {
        "if": SDK_IF,
        "working-directory": "beamfall-plugin-podcasts",
        "run": "go mod edit -replace github.com/beamfall/plugin-sdk=../beamfall-plugin-sdk",
    },
}
JOB_SPECS = {
    "fast-conformance": {
        "fields": {"if": None, "name": "fast conformance"},
        "sdk": True,
        "steps": {
            "gofmt": {
                "working-directory": "beamfall-plugin-podcasts",
                "run": "gofmt -l . | tee /dev/stderr | (! grep -q .)",
            },
            "vet": {
                "working-directory": "beamfall-plugin-podcasts",
                "run": "go vet ./...",
            },
            "gate contracts": {
                "working-directory": "beamfall-plugin-podcasts",
                "run": "script/gate.sh contracts",
            },
            "govulncheck": {
                "working-directory": "beamfall-plugin-podcasts",
                "run": "go -C tools tool govulncheck -C .. ./...",
            },
            "tests and per-package coverage": {
                "working-directory": "beamfall-plugin-podcasts",
                "run": "python3 .github/workflows/coverage_gate.py",
            },
            "golangci-lint": {
                "uses": "golangci/golangci-lint-action@25e2cdc5eb1d7a04fdc45ff538f1a00e960ae128",
                "with": {
                    "version": "v2.12.2",
                    "working-directory": "beamfall-plugin-podcasts",
                },
            },
        },
    },
    "race": {
        "fields": {"if": None},
        "sdk": True,
        "steps": {
            "race": {
                "working-directory": "beamfall-plugin-podcasts",
                "run": "script/gate.sh race",
            }
        },
    },
    "conformance": {
        "fields": {
            "if": "always()",
            "name": "conformance",
            "needs": "[fast-conformance, race]",
        },
        "steps": {
            "Require fast and race jobs": {
                "env": {
                    "FAST_RESULT": "${{ needs.fast-conformance.result }}",
                    "RACE_RESULT": "${{ needs.race.result }}",
                },
                "run": 'test "$FAST_RESULT" = success\ntest "$RACE_RESULT" = success',
            }
        },
    },
    "fuzz": {
        "fields": {"if": FUZZ_IF, "timeout-minutes": "10"},
        "sdk": True,
        "steps": {
            "bounded OPML fuzz": {
                "working-directory": "beamfall-plugin-podcasts",
                "env": {"FUZZ_TIME": "30s"},
                "run": "script/gate.sh fuzz",
            }
        },
    },
}


def uncommented(text: str) -> list[str]:
    return [line for line in text.splitlines() if not line.lstrip().startswith("#")]


def jobs(workflow: str) -> dict[str, str]:
    lines = uncommented(workflow)
    try:
        start = lines.index("jobs:") + 1
    except ValueError:
        return {}
    found: dict[str, list[str]] = {}
    current = ""
    for line in lines[start:]:
        match = re.match(r"^  ([A-Za-z0-9_-]+):\s*$", line)
        if match:
            current = match.group(1)
            found[current] = [line]
        elif current:
            found[current].append(line)
    return {name: "\n".join(lines) for name, lines in found.items()}


def steps(job: str) -> list[str]:
    found: list[list[str]] = []
    current: list[str] = []
    for line in uncommented(job):
        if line.startswith("      - "):
            if current:
                found.append(current)
            current = [line]
        elif current:
            current.append(line)
    if current:
        found.append(current)
    return ["\n".join(lines) for lines in found]


def values(block: str, indent: int, key: str) -> list[str]:
    return re.findall(rf"^{' ' * indent}{re.escape(key)}:\s*(.*?)\s*$", block, re.MULTILINE)


def name(step: str) -> str:
    found = re.findall(r"^      - name:\s*(.*?)\s*$", step, re.MULTILINE)
    return found[0] if len(found) == 1 else ""


def mapping(step: str, key: str) -> dict[str, str]:
    lines = uncommented(step)
    starts = [index for index, line in enumerate(lines) if line == f"        {key}:"]
    if len(starts) != 1:
        return {}
    result: dict[str, str] = {}
    for line in lines[starts[0] + 1 :]:
        if re.match(r"^        [A-Za-z0-9_-]+:", line):
            break
        match = re.match(r"^          ([A-Za-z0-9_-]+):\s*(.*?)\s*$", line)
        if match:
            result[match.group(1)] = match.group(2)
    return result


def runs(step: str) -> list[str]:
    lines = uncommented(step)
    result: list[str] = []
    for index, line in enumerate(lines):
        match = re.match(r"^        run:\s*(.*?)\s*$", line)
        if not match:
            continue
        if match.group(1) not in {"|", "|-", ">", ">-"}:
            result.append(match.group(1))
            continue
        body: list[str] = []
        for body_line in lines[index + 1 :]:
            if not body_line.startswith("          "):
                break
            body.append(body_line[10:])
        result.append("\n".join(body).rstrip())
    return result


def function(script: str, target: str) -> str:
    lines = script.splitlines()
    try:
        start = lines.index(f"{target}() {{") + 1
    except ValueError:
        return ""
    for end in range(start, len(lines)):
        if lines[end] == "}":
            return "\n".join(uncommented("\n".join(lines[start:end])))
    return ""


def arm(script: str, target: str) -> str:
    lines = script.splitlines()
    try:
        start = lines.index(f"  {target})") + 1
    except ValueError:
        return ""
    body: list[str] = []
    for line in lines[start:]:
        if re.match(r"^  [A-Za-z0-9_-]+\)$", line) or line == "esac":
            break
        body.append(line)
    return "\n".join(uncommented("\n".join(body)))


def validate_step(errors: list[str], job_name: str, step: str, spec: dict) -> None:
    step_name = name(step)
    for key in ("if", "uses", "working-directory"):
        expected = [] if key not in spec else [spec[key]]
        if values(step, 8, key) != expected:
            errors.append(f"{job_name}/{step_name} {key} contract changed")
    for key in ("env", "with"):
        if key in spec and mapping(step, key) != spec[key]:
            errors.append(f"{job_name}/{step_name} {key} contract changed")
    if "run" in spec and runs(step) != [spec["run"]]:
        errors.append(f"{job_name}/{step_name} run contract changed")


def workflow_errors(workflow: str) -> list[str]:
    errors: list[str] = []
    active = "\n".join(uncommented(workflow))
    parsed = jobs(workflow)
    for trigger in ("  pull_request:", "  schedule:", "  workflow_dispatch:"):
        if trigger not in active:
            errors.append(f"missing trigger {trigger.strip()}")

    for job_name, spec in JOB_SPECS.items():
        job = parsed.get(job_name, "")
        if not job:
            errors.append(f"missing job {job_name}")
            continue
        for key, expected in spec["fields"].items():
            wanted = [] if expected is None else [expected]
            if values(job, 4, key) != wanted:
                errors.append(f"{job_name} {key} contract changed")
        if values(job, 4, "continue-on-error"):
            errors.append(f"{job_name} may not continue on error")

        parsed_steps = steps(job)
        allowed_conditions = set(SDK_STEPS) if spec.get("sdk") else set()
        for step in parsed_steps:
            step_name = name(step)
            if step_name not in allowed_conditions and values(step, 8, "if"):
                errors.append(f"{job_name}/{step_name or '<unnamed>'} must be unconditional")
            if values(step, 8, "continue-on-error"):
                errors.append(f"{job_name}/{step_name or '<unnamed>'} may not continue on error")

        expected_steps = dict(spec["steps"])
        if spec.get("sdk"):
            expected_steps.update(SDK_STEPS)
        for step_name, step_spec in expected_steps.items():
            matches = [step for step in parsed_steps if name(step) == step_name]
            if len(matches) != 1:
                errors.append(f"{job_name}/{step_name} missing or duplicated")
            else:
                validate_step(errors, job_name, matches[0], step_spec)

    fast_runs = [command for step in steps(parsed.get("fast-conformance", "")) for command in runs(step)]
    coverage = "python3 .github/workflows/coverage_gate.py"
    if fast_runs.count(coverage) != 1:
        errors.append("fast-conformance must run coverage exactly once")
    if any(
        (re.search(r"(^|\s)(?:\./)?script/gate\.sh(\s|$)", command) and command != "script/gate.sh contracts")
        or ("coverage_gate.py" in command and command != coverage)
        or re.search(r"(^|\s)go test(\s|$)", command)
        for command in fast_runs
    ):
        errors.append("fast-conformance duplicates the coverage test pass")
    return errors


def contract_errors(workflow: str, gate: str, fuzz_source: str) -> list[str]:
    errors = workflow_errors(workflow)
    if "go test -race -count=1 ./..." not in arm(gate, "race"):
        errors.append("race gate mode changed")
    fuzz = function(gate, "run_fuzz")
    for label, text in (
        ("30-second default fuzz budget", 'fuzz_time="${FUZZ_TIME:-30s}"'),
        ("fuzz target discovery", "go test -list '^FuzzParseOPML$' ."),
        ("exact fuzz execution", "-fuzz '^FuzzParseOPML$'"),
        ("positive fuzz budget", "^([1-9][0-9]*)(s|x)$"),
    ):
        if text not in fuzz:
            errors.append(label)
    if "run_fuzz" not in arm(gate, "fuzz"):
        errors.append("fuzz gate mode changed")
    if "python3 .github/workflows/coverage_gate.py" not in function(gate, "run_coverage"):
        errors.append("coverage gate mode changed")
    if not re.search(r"^func FuzzParseOPML\(", fuzz_source, re.MULTILINE):
        errors.append("required FuzzParseOPML target missing")
    return errors


def tool_pin_errors(root_go_mod: str, root_go_sum: str, tools_go_mod: str, tools_go_sum: str) -> list[str]:
    errors: list[str] = []
    directives = [line.strip() for line in tools_go_mod.splitlines() if not line.lstrip().startswith("//")]
    if directives.count("tool golang.org/x/vuln/cmd/govulncheck") != 1:
        errors.append("tools/go.mod must declare govulncheck exactly once")
    vuln_requirements = [line for line in directives if re.match(r"^golang\.org/x/vuln\s", line)]
    if vuln_requirements != ["golang.org/x/vuln v1.6.0 // indirect"]:
        errors.append("tools/go.mod must pin golang.org/x/vuln v1.6.0")
    if any(re.match(r"^(replace|exclude)\s+golang\.org/x/vuln(?:\s|$)", line) for line in directives):
        errors.append("tools/go.mod must not replace or exclude the reviewed golang.org/x/vuln module")
    if re.search(r"^\s*golang\.org/x/vuln\s", root_go_mod, re.MULTILINE):
        errors.append("root go.mod must not depend on golang.org/x/vuln")
    if re.search(r"^golang\.org/x/vuln\s", root_go_sum, re.MULTILINE):
        errors.append("root go.sum must not contain golang.org/x/vuln checksums")
    expected_sums = {
        "golang.org/x/vuln v1.6.0 h1:FeMO9Rm/HwyduOztbvKcOw+zvDEPr4I4aQNSfevFcKY=",
        "golang.org/x/vuln v1.6.0/go.mod h1:bWlG2493/sjR7ksvicBgMrznH3eYQEyK8ifUYBrqUbg=",
    }
    actual_sums = {line for line in tools_go_sum.splitlines() if line.startswith("golang.org/x/vuln ")}
    if actual_sums != expected_sums:
        errors.append("tools/go.sum must contain only the reviewed golang.org/x/vuln v1.6.0 checksums")
    return errors


def replace_nth(text: str, old: str, new: str, index: int) -> str:
    start = -1
    for _ in range(index + 1):
        start = text.find(old, start + 1)
        if start < 0:
            raise AssertionError(f"mutation occurrence {index} not found")
    return text[:start] + new + text[start + len(old) :]


class GateContractTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.workflow = WORKFLOW.read_text(encoding="utf-8")
        cls.gate = GATE.read_text(encoding="utf-8")
        cls.source = FUZZ_SOURCE.read_text(encoding="utf-8")
        cls.root_go_mod = ROOT_GO_MOD.read_text(encoding="utf-8")
        cls.root_go_sum = ROOT_GO_SUM.read_text(encoding="utf-8")
        cls.tools_go_mod = TOOLS_GO_MOD.read_text(encoding="utf-8")
        cls.tools_go_sum = TOOLS_GO_SUM.read_text(encoding="utf-8")

    def reject(self, workflow: str | None = None, gate: str | None = None, source: str | None = None) -> None:
        self.assertTrue(contract_errors(workflow or self.workflow, gate or self.gate, source or self.source))

    def mutate(self, old: str, new: str) -> str:
        self.assertIn(old, self.workflow)
        return self.workflow.replace(old, new, 1)

    def test_live_contract(self) -> None:
        self.assertEqual(contract_errors(self.workflow, self.gate, self.source), [])

    def test_govulncheck_tool_pin(self) -> None:
        self.assertEqual(
            tool_pin_errors(self.root_go_mod, self.root_go_sum, self.tools_go_mod, self.tools_go_sum),
            [],
        )

    def test_govulncheck_tool_pin_rejects_mutable_selectors(self) -> None:
        self.reject(
            self.mutate(
                "go -C tools tool govulncheck -C .. ./...",
                "go run golang.org/x/vuln/cmd/govulncheck@latest ./...",
            )
        )
        unpinned = self.tools_go_mod.replace("golang.org/x/vuln v1.6.0", "golang.org/x/vuln v1.7.0")
        self.assertTrue(tool_pin_errors(self.root_go_mod, self.root_go_sum, unpinned, self.tools_go_sum))

    def test_aggregator_cannot_detach_or_accept_failed_results(self) -> None:
        self.reject(self.mutate("needs: [fast-conformance, race]", "needs: [fast-conformance]"))
        self.reject(self.mutate('test "$RACE_RESULT" = success', 'test -n "$RACE_RESULT"'))

    def test_jobs_cannot_skip_or_continue(self) -> None:
        mutations = {
            "fast-conformance": "  fast-conformance:\n    name:",
            "race": "  race:\n    runs-on:",
            "conformance": "    if: always()",
            "fuzz": f"    if: {FUZZ_IF}",
        }
        for job, old in mutations.items():
            with self.subTest(job=job, mode="skip"):
                new = (
                    "    if: github.event_name == 'never'"
                    if job in {"conformance", "fuzz"}
                    else old.replace("\n", "\n    if: github.event_name == 'never'\n", 1)
                )
                self.reject(self.mutate(old, new))
            with self.subTest(job=job, mode="continue"):
                self.reject(self.mutate(f"  {job}:\n", f"  {job}:\n    continue-on-error: true\n"))

    def test_critical_steps_cannot_skip_continue_or_hide_commands(self) -> None:
        cases = (
            ("gofmt", "working-directory", "run", "gofmt -l . | tee /dev/stderr | (! grep -q .)"),
            ("vet", "working-directory", "run", "go vet ./..."),
            ("gate contracts", "working-directory", "run", "script/gate.sh contracts"),
            ("govulncheck", "working-directory", "run", "go -C tools tool govulncheck -C .. ./..."),
            ("tests and per-package coverage", "working-directory", "run", "python3 .github/workflows/coverage_gate.py"),
            ("golangci-lint", "uses", "uses", "golangci/golangci-lint-action@25e2cdc5eb1d7a04fdc45ff538f1a00e960ae128"),
            ("race", "working-directory", "run", "script/gate.sh race"),
            ("Require fast and race jobs", "env", None, None),
            ("bounded OPML fuzz", "working-directory", "run", "script/gate.sh fuzz"),
        )
        for step, next_key, command_key, command in cases:
            prefix = f"      - name: {step}\n"
            with self.subTest(step=step, mode="skip"):
                self.reject(self.mutate(prefix + f"        {next_key}:", prefix + "        if: github.event_name == 'never'\n" + f"        {next_key}:"))
            with self.subTest(step=step, mode="continue"):
                self.reject(self.mutate(prefix, prefix + "        continue-on-error: true\n"))
            with self.subTest(step=step, mode="comment"):
                if command:
                    replacement = "actions/checkout@v4" if command_key == "uses" else "echo bypassed"
                    self.reject(self.mutate(f"        {command_key}: {command}", f"        {command_key}: {replacement}\n        # {command_key}: {command}"))
                else:
                    old = '        run: |\n          test "$FAST_RESULT" = success\n          test "$RACE_RESULT" = success'
                    new = '        run: echo bypassed\n        # run: |\n        #   test "$FAST_RESULT" = success\n        #   test "$RACE_RESULT" = success'
                    self.reject(self.mutate(old, new))

    def test_required_fast_stages_cannot_be_deleted(self) -> None:
        fast_steps = steps(jobs(self.workflow)["fast-conformance"])
        for step_name in ("gofmt", "vet", "govulncheck", "golangci-lint"):
            with self.subTest(step=step_name):
                block = next(step for step in fast_steps if name(step) == step_name)
                self.assertIn(block, self.workflow)
                self.reject(self.workflow.replace(block, "", 1))

    def test_wrapper_duplicate_test_passes_are_rejected(self) -> None:
        anchor = "      - name: golangci-lint\n"
        for command in (
            "script/gate.sh",
            "script/gate.sh gate",
            "script/gate.sh coverage",
            "./script/gate.sh coverage",
            "go test ./...",
        ):
            with self.subTest(command=command):
                duplicate = f"      - name: duplicate test pass\n        working-directory: beamfall-plugin-podcasts\n        run: {command}\n"
                self.reject(self.mutate(anchor, duplicate + anchor))

    def test_sdk_override_removal_breaks_fast_race_and_fuzz(self) -> None:
        for index, job in enumerate(("fast-conformance", "race", "fuzz")):
            for label, block in (("checkout", SDK_CHECKOUT), ("use", SDK_USE)):
                with self.subTest(job=job, step=label):
                    self.reject(replace_nth(self.workflow, block, "", index))

    def test_fuzz_budget_timeout_and_target_are_pinned(self) -> None:
        self.reject(self.mutate("    timeout-minutes: 10", "    timeout-minutes: 0"))
        self.reject(self.mutate("          FUZZ_TIME: 30s", "          FUZZ_TIME: 0s"))
        self.reject(gate=self.gate.replace('fuzz_time="${FUZZ_TIME:-30s}"', 'fuzz_time="${FUZZ_TIME:-0s}"'))
        self.reject(source=self.source.replace("func FuzzParseOPML(", "func ParseOPMLFuzz("))


if __name__ == "__main__":
    unittest.main()
