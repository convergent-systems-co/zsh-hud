// Package input interprets the raw terminal input stream into bytes to forward
// to the shell and semantic HUD actions (scroll, copy-mode navigation, yank).
// It is pure and holds no terminal geometry: main applies the actions against
// the engine/compositor state.
package input
