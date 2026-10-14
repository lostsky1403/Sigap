/**
 * Ambient type wiring for the unit test layer.
 *
 * `svelte-check` type-checks this project's test files, so the matchers that
 * `vitest-setup.ts` registers at runtime must also be visible to the type
 * checker. Importing `@testing-library/jest-dom/vitest` merges the DOM matcher
 * declarations into Vitest's `Assertion` interface, which is why this file
 * exists rather than a hand-written list of matchers.
 */
import '@testing-library/jest-dom/vitest';
