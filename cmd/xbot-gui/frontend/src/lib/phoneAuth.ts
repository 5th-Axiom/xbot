export const countryCodeOptions = [
  { value: "+86", label: "CN +86" },
  { value: "+1", label: "US +1" },
  { value: "+44", label: "UK +44" },
  { value: "+81", label: "JP +81" },
  { value: "+82", label: "KR +82" },
  { value: "+65", label: "SG +65" },
  { value: "+852", label: "HK +852" },
];

export function normalizePhoneNumber(value: string): string {
  return value.replace(/\D+/g, "");
}
