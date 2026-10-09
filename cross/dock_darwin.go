package main

// Mac: 窓を隠したら Dock からも消えて、メニューバーのアイコンだけになる (Windows がトレイだけになるのと同じ)。
// 窓を出すと Dock に戻る。Wails には起動時の設定しかないので、AppKit を直接呼ぶ。

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void lumiSetDockVisible(int show) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (show) {
			[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
			[NSApp activateIgnoringOtherApps:YES];
		} else {
			[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		}
	});
}
*/
import "C"

func setDockVisible(show bool) {
	v := 0
	if show {
		v = 1
	}
	C.lumiSetDockVisible(C.int(v))
}
