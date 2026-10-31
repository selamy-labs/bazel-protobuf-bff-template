import type { ReactElement } from "react";

/** The label is what makes a test credential recognizable. The server accepts it only in dev mode. */
const prefix = "Bearer test-identity.";

export function testCredential(subject: string): string | null {
  return subject === "" ? null : prefix + subject;
}

export function TestIdentity({
  subject,
  onChange,
}: {
  subject: string;
  onChange: (subject: string) => void;
}): ReactElement {
  return (
    <aside data-testid="test-identity">
      <label>
        Test identity: development only, never deployed
        <input name="subject" value={subject} onChange={(event) => onChange(event.target.value)} />
      </label>
    </aside>
  );
}
