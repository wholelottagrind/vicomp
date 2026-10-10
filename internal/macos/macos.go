// Package macos wraps the few macOS integrations vicomp uses.
package macos

import (
	"fmt"
	"os/exec"
	"strings"
)

// jxa runs a JavaScript for Automation snippet. The snippet's run(argv)
// function receives args, so paths never need escaping into the script.
func jxa(script string, args ...string) error {
	out, err := exec.Command("osascript", append([]string{"-l", "JavaScript", "-e", script}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

// CopyFile puts the file itself on the clipboard, like ⌘C in Finder, so it
// can be pasted into Telegram or any other app.
func CopyFile(path string) error {
	return jxa(`function run(argv) {
		ObjC.import("AppKit");
		var pb = $.NSPasteboard.generalPasteboard;
		pb.clearContents;
		if (!pb.writeObjects($.NSArray.arrayWithObject($.NSURL.fileURLWithPath(argv[0])))) {
			throw new Error("couldn't write to the clipboard");
		}
	}`, path)
}

// CopyText puts text on the clipboard.
func CopyText(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// Reveal selects the file in a Finder window.
func Reveal(path string) error {
	return exec.Command("open", "-R", path).Run()
}

// Trash moves the file to the Trash (recoverable, unlike deleting).
func Trash(path string) error {
	return jxa(`function run(argv) {
		ObjC.import("Foundation");
		var ok = $.NSFileManager.defaultManager.trashItemAtURLResultingItemURLError(
			$.NSURL.fileURLWithPath(argv[0]), null, null);
		if (!ok) { throw new Error("couldn't move to Trash"); }
	}`, path)
}
