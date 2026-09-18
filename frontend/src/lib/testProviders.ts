export function isTestProviderName(name: string): boolean {
  const lower = name.toLowerCase();
  return lower.startsWith("testserve") || lower.startsWith("test-");
}

export function isTestUserEmail(email: string): boolean {
  return email.toLowerCase().endsWith("@local.test");
}
