"""
Sample source file for byom-workspace demo.

The BYOM agent reads README.md as project context and writes
its analysis to docs/byom-agent-notes.md. This file exists
to give the project a realistic structure that the agent
can reference in its output.
"""


def process(items: list) -> list:
    """Return a filtered and sorted view of items."""
    return sorted(x for x in items if x is not None)


def summarize(data: dict) -> str:
    """Produce a one-line summary of a data dict."""
    keys = sorted(data.keys())
    return f"{len(keys)} fields: " + ", ".join(keys[:5])
