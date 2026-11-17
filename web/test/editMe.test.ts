import assert from 'node:assert/strict';
import { test } from 'node:test';
import type { FetchError } from 'ofetch';
import type { MeInput, User } from '../app/types/items.ts';
import { editMe } from '../app/utils/editMe.ts';

const account = (updated_at: string, snippets: string[]) =>
  ({ updated_at, snippets: snippets.map((name) => ({ name, content: '' })) }) as User;
const stale = () => Promise.reject({ status: 409 } as FetchError);
const add = (me: User): MeInput => ({ snippets: [...me.snippets, { name: 'mine', content: '' }] });

test('sends the change with the updated_at it was made on', async () => {
  const sent: MeInput[] = [];
  const result = await editMe(
    account('t1', ['a']),
    add,
    async (body) => (sent.push(body), account('t2', ['a', 'mine'])),
    () => assert.fail('no read'),
  );
  assert.deepEqual(sent, [
    {
      snippets: [
        { name: 'a', content: '' },
        { name: 'mine', content: '' },
      ],
      updated_at: 't1',
    },
  ]);
  assert.deepEqual(result, { me: account('t2', ['a', 'mine']), saved: true });
});

test('on a 409, redoes the change once on the account read again', async () => {
  const sent: MeInput[] = [];
  const result = await editMe(
    account('t1', ['a']),
    add,
    (body) => (sent.push(body), sent.length === 1 ? stale() : Promise.resolve(account('t3', ['a', 'b', 'mine']))),
    async () => account('t2', ['a', 'b']),
  );
  assert.equal(sent[1]!.updated_at, 't2');
  assert.deepEqual(
    sent[1]!.snippets?.map((s) => s.name),
    ['a', 'b', 'mine'],
  );
  assert.equal(result.saved, true);
});

test('a second 409 keeps the fresh account, nothing saved', async () => {
  let reads = 0;
  const result = await editMe(account('t1', ['a']), add, stale, async () => account(`t${++reads + 1}`, ['b']));
  assert.equal(reads, 2);
  assert.deepEqual(result, { me: account('t3', ['b']), saved: false });
});

test('another error is thrown as is', async () => {
  await assert.rejects(
    editMe(
      account('t1', []),
      add,
      () => Promise.reject({ status: 500 }),
      () => assert.fail('no read'),
    ),
    { status: 500 },
  );
});
