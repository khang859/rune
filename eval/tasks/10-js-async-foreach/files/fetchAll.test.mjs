import test from "node:test";
import assert from "node:assert/strict";

import { fetchAll } from "./fetchAll.js";

const fetchOne = (id) =>
  new Promise((resolve) => setTimeout(() => resolve(id * 2), 10));

test("fetchAll awaits all fetches and preserves input order", async () => {
  const out = await fetchAll([1, 2, 3, 4], fetchOne);
  assert.deepEqual(out, [2, 4, 6, 8]);
});

test("fetchAll on empty input returns empty array", async () => {
  const out = await fetchAll([], fetchOne);
  assert.deepEqual(out, []);
});

test("fetchAll preserves order even when later items resolve faster", async () => {
  const variableFetch = (id) =>
    new Promise((resolve) => setTimeout(() => resolve(id), (5 - id) * 10));
  const out = await fetchAll([1, 2, 3, 4], variableFetch);
  assert.deepEqual(out, [1, 2, 3, 4]);
});
