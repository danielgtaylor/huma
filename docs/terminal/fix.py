#!/usr/bin/env python3

import json
import sys

lines = open(sys.argv[1]).readlines()
for i in range(len(lines)):
	if '$ "]' in lines[i]:
		if i < len(lines) - 1:
			if '"\\r\\n"]' in lines[i+1]:
				lines[i] = lines[i].replace("$ ", "")
			if '"#"' in lines[i+1]:
				lines[i] = lines[i].replace("$ ", "\\u001b[39;2m")
	lines[i] = lines[i].replace("\\r\\n", "\\r\\n\\u001b[0m")

# Colorize the prompts that are left.
for i in range(len(lines)):
	lines[i] = lines[i].replace("$ ", "\\u001b[34m$\\u001b[0m ")

# Ellipsize similar lines
packages = []
package_row = None
package_del = []
for i in range(len(lines)):
	if "go: added github.com/" in lines[i]:
		# Decode line as JSON
		data = json.loads(lines[i])
		packages += data[2].split("\r\n")
		if package_row is None:
			package_row = data
		package_del += [i]

if package_row:
	packages = packages[:3] + ["..."] + packages[-3:]
	package_row[2] = "\r\n".join(packages)
	lines[package_del[0]] = json.dumps(package_row) + "\n"

for i in reversed(package_del[1:]):
	del lines[i]

print("".join(lines[:-2]))
