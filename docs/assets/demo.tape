# Renders docs/assets/demo.gif with VHS (https://github.com/charmbracelet/vhs).
# Run through the demo workflow, which installs a real release first:
#   gh workflow run demo.yml -f version=v0.x.y

Output docs/assets/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1280
Set Height 640
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 35ms

Hide
Type "export PS1='$ ' && clear"
Enter
Show

# The TUI. Number keys pick screens in the order of the tab bar:
# 1 Status, 2 Setup, 3 Agents, 4 Update, 5 Doctor, 6 Logs.
# Wait+Screen fails the recording if the expected text never shows up.
Type "ohmylaya"
Sleep 400ms
Enter
Wait+Screen@20s /Status/
Sleep 2.5s
Type "3"
Wait+Screen@20s /.claude.json/
Sleep 2.5s
Type "5"
Wait+Screen@60s /runtime-deps/
Sleep 3s
Type "q"
Wait@10s /\$\s*$/

# One real tool call: triage three issues against your own labels.
Type "cat triage.json | ohmylaya ask classify | jq -c '.results[] | {id, label, action}'"
Sleep 600ms
Enter
Wait+Screen@60s /"#43"/
Sleep 4s
