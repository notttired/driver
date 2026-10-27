/** Client-side preview of the server's slug normalisation: lowercase,
 *  [a-z0-9] kept, every other run collapsed to one dash, ends trimmed. */
export function slugify(s: string): string {
  let out = "";
  let lastDash = true;
  for (const ch of s.toLowerCase()) {
    if ((ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9")) {
      out += ch;
      lastDash = false;
    } else if (!lastDash) {
      out += "-";
      lastDash = true;
    }
  }
  return out.replace(/^-+|-+$/g, "");
}

export const SLUG_RE = /^[a-z0-9][a-z0-9-]{0,63}$/;
