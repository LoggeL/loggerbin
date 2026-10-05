import { test } from "node:test";
import assert from "node:assert/strict";
import {
  encryptText,
  decryptText,
  decode,
  encode,
} from "../internal/web/static/crypto.js";

test("browser crypto round-trips Unicode and script-looking text", async () => {
  const text = "Hello 🦊\n</script><img src=x onerror=alert(1)>\n日本語";
  const result = await encryptText(text);
  assert.equal(decode(result.key, 32, 32).length, 32);
  assert.equal(await decryptText(result.envelope, result.key), text);
  assert.equal("key" in result.envelope, false);
  assert.equal(JSON.stringify(result.envelope).includes(text), false);
});
test("tampering, a wrong key and a changed nonce are rejected", async () => {
  const result = await encryptText("authenticated text");
  const other = await encryptText("other");
  await assert.rejects(decryptText(result.envelope, other.key));
  const damaged = decode(result.envelope.ciphertext, 17, 65552);
  damaged[0] ^= 1;
  await assert.rejects(
    decryptText(
      { ...result.envelope, ciphertext: encode(damaged) },
      result.key,
    ),
  );
  await assert.rejects(
    decryptText(
      { ...result.envelope, nonce: other.envelope.nonce },
      result.key,
    ),
  );
});
test("empty, oversized and malformed inputs are rejected", async () => {
  await assert.rejects(encryptText("   "));
  await assert.rejects(encryptText("🦊", 3));
  await assert.rejects(encryptText("x".repeat(65537)));
  assert.throws(() => decode("not/base64==", 32, 32));
  const r = await encryptText("text");
  await assert.rejects(decryptText({ ...r.envelope, version: 2 }, r.key));
  await assert.rejects(decryptText(r.envelope, "A".repeat(44)));
});
test("a fresh key and nonce are generated for each paste", async () => {
  const a = await encryptText("same");
  const b = await encryptText("same");
  assert.notEqual(a.key, b.key);
  assert.notEqual(a.envelope.nonce, b.envelope.nonce);
  assert.notEqual(a.envelope.ciphertext, b.envelope.ciphertext);
});
