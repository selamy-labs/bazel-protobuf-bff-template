// Template-owned Playwright configuration for reproducible BFF web video evidence (#27).
// Derivatives can import createVideoPlaywrightConfig from ./video-evidence.mjs without copying a
// second recording framework. Strictly testonly tooling; never imported by production bundles.
import { createVideoPlaywrightConfig } from "./video-evidence.mjs";

export default createVideoPlaywrightConfig();
