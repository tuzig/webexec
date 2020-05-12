#!/usr/bin/env python3
import subprocess

if __name__ == "__main__":
    c = subprocess.run(["tmux", "list-sessions", "-F",
                        "#{session_id}:#{session_attached}"],
                       capture_output=True, text=True)
    for line in c.stdout.split("\n"):
        if line:
            id, clients = line.split(":")
            if clients == '0':
                print(f"Killing session {id}")
                subprocess.run(["tmux", "kill-session", "-t", id])
