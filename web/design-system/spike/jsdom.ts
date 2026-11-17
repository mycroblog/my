import globalJsdom from "global-jsdom";

globalJsdom(undefined, { url: "http://localhost/", pretendToBeVisual: true });
(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// No animation engine here: Motion finishes every animation at once, so an exit completes.
const { MotionGlobalConfig } = await import("motion/react");
MotionGlobalConfig.skipAnimations = true;
