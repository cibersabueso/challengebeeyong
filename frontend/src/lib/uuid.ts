const UUID_V4_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

export function isUuidV4(s: string): boolean {
  return UUID_V4_REGEX.test(s);
}

export function newUuidV4(): string {
  return crypto.randomUUID();
}
