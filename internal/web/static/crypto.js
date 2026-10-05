const aad = new TextEncoder().encode("loggerbin:v1");

export function encode(bytes) {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 4096)
    binary += String.fromCharCode(...bytes.subarray(i, i + 4096));
  return btoa(binary)
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replaceAll("=", "");
}

export function decode(value, min, max) {
  if (
    typeof value !== "string" ||
    !/^[A-Za-z0-9_-]+$/.test(value) ||
    value.length > Math.ceil((max * 4) / 3)
  )
    throw new Error("Invalid encrypted data");
  const binary = atob(value.replaceAll("-", "+").replaceAll("_", "/"));
  const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0));
  if (bytes.length < min || bytes.length > max || encode(bytes) !== value)
    throw new Error("Invalid encrypted data");
  return bytes;
}

export async function encryptText(text, maxBytes = 65536) {
  if (typeof text !== "string" || !text.trim())
    throw new Error("Enter some text first.");
  const plain = new TextEncoder().encode(text);
  if (plain.length > maxBytes)
    throw new Error(
      `Paste exceeds the ${maxBytes.toLocaleString()} byte limit.`,
    );
  const rawKey = crypto.getRandomValues(new Uint8Array(32));
  const nonce = crypto.getRandomValues(new Uint8Array(12));
  const key = await crypto.subtle.importKey("raw", rawKey, "AES-GCM", false, [
    "encrypt",
  ]);
  const ciphertext = new Uint8Array(
    await crypto.subtle.encrypt(
      { name: "AES-GCM", iv: nonce, additionalData: aad, tagLength: 128 },
      key,
      plain,
    ),
  );
  const encodedKey = encode(rawKey);
  rawKey.fill(0);
  plain.fill(0);
  return {
    key: encodedKey,
    envelope: {
      version: 1,
      nonce: encode(nonce),
      ciphertext: encode(ciphertext),
    },
  };
}

export async function decryptText(envelope, encodedKey, maxBytes = 65536) {
  if (envelope?.version !== 1) throw new Error("Unsupported paste format.");
  const rawKey = decode(encodedKey, 32, 32);
  const nonce = decode(envelope.nonce, 12, 12);
  const ciphertext = decode(envelope.ciphertext, 17, maxBytes + 16);
  const key = await crypto.subtle.importKey("raw", rawKey, "AES-GCM", false, [
    "decrypt",
  ]);
  rawKey.fill(0);
  let plain;
  try {
    plain = new Uint8Array(
      await crypto.subtle.decrypt(
        { name: "AES-GCM", iv: nonce, additionalData: aad, tagLength: 128 },
        key,
        ciphertext,
      ),
    );
  } catch {
    throw new Error(
      "This link cannot decrypt the paste. It may have the wrong key or damaged data.",
    );
  }
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(plain);
  } finally {
    plain.fill(0);
  }
}
