/**
 * package: core / build
 * type:    logic
 * job:     name the build this bundle is, stamped by vite at compile time
 * limits:  a constant; the value comes from the Makefile (-> vite.config.ts)
 */

declare const __EXPLORER_VERSION__: string;

/** VERSION is what `git describe` gave the build, or "dev" for an unstamped one. */
export const VERSION: string = __EXPLORER_VERSION__;
