import type { FetchError } from 'ofetch';
import type { MeInput, User } from '../types/items.ts';

/**
 * Writes one change of the account's settings, guarded by its updated_at so it never overwrites a change made
 * meanwhile (an agent through the MCP, another tab). On a 409 it reads the account again and redoes the change
 * once on the fresh one, like the MCP's editMe. saved is false when that second write is refused too: me is then
 * the fresh account, nothing written.
 */
export async function editMe(
  me: User,
  edit: (me: User) => MeInput,
  send: (body: MeInput) => Promise<User>,
  read: () => Promise<User>,
): Promise<{ me: User; saved: boolean }> {
  for (let tries = 0; ; tries++) {
    try {
      return { me: await send({ ...edit(me), updated_at: me.updated_at }), saved: true };
    } catch (error) {
      if ((error as FetchError).status !== 409) throw error;
      me = await read();
      if (tries > 0) return { me, saved: false };
    }
  }
}
