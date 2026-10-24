import { encodePayload } from '@argus-labs/sdk'
import { afterEach, expect, test, vi } from 'vitest'
import { ShardConnection, on } from './shard_connection'
import type { Message } from './shard_connection'

// The SDK's real requests, caught by a fake fetch.

const origin = 'http://localhost:5173'
const where = { gatewayUrl: `${origin}/edge/org`, project: 'demo', shardId: 'arena' }
const service = `${origin}/edge/org/demo/arena/worldengine.cardinal.v1.CardinalService`
const arena = { project: 'demo', serviceId: 'arena' }

interface Sample {
  Tick: bigint
  Name: string
  X: number
}
const sample: Sample = { Tick: 9007199254740993n, Name: 'Échø ✦', X: 1.5 }
const SampleSync: Message<Sample> = { name: 'sample_sync' }
const OtherSync: Message<Sample> = { name: 'other_sync' }
const MoveCommand: Message<Sample> = { name: 'move' }

// Just enough protobuf: every field used here is length-delimited.
const varint = (n: number): number[] => (n < 0x80 ? [n] : [(n & 0x7f) | 0x80, ...varint(n >>> 7)])
const concat = (...parts: Uint8Array[]) => new Uint8Array(parts.flatMap((p) => [...p]))
const field = (n: number, value: Uint8Array | string) => {
  const bytes = typeof value === 'string' ? new TextEncoder().encode(value) : value
  return concat(new Uint8Array([...varint((n << 3) | 2), ...varint(bytes.length)]), bytes)
}
// A Connect stream message: a flags byte, the big-endian length, then the message.
const envelope = (message: Uint8Array) => {
  const head = new Uint8Array(5)
  new DataView(head.buffer).setUint32(1, message.length)
  return concat(head, message)
}

/** A message's fields, by number. */
function fields(bytes: Uint8Array = new Uint8Array()): Map<number, Uint8Array[]> {
  const out = new Map<number, Uint8Array[]>()
  let i = 0
  const read = () => {
    let n = 0
    for (let shift = 0; ; shift += 7) {
      const b = bytes[i++] ?? 0
      n |= (b & 0x7f) << shift
      if (b < 0x80) return n
    }
  }
  while (i < bytes.length) {
    const tag = read()
    if ((tag & 7) !== 2) throw new Error(`field ${tag >>> 3} is not length-delimited`)
    const length = read()
    out.set(tag >>> 3, [...(out.get(tag >>> 3) ?? []), bytes.subarray(i, i + length)])
    i += length
  }
  return out
}
const one = (bytes: Uint8Array | undefined, n: number) => fields(bytes).get(n)?.[0]
const text = (bytes: Uint8Array | undefined) => new TextDecoder().decode(bytes)
// worldengine.micro.v1.ServiceAddress: project = 4, service_id = 5.
const address = (bytes: Uint8Array | undefined) => ({
  project: text(one(bytes, 4)),
  serviceId: text(one(bytes, 5)),
})
// The EventSubscriptions (field 1) of a subscribe request: address = 1, events = 2.
const subscriptions = (request: Uint8Array | undefined) =>
  (fields(request).get(1) ?? []).map((s) => ({
    address: address(one(s, 1)),
    events: (fields(s).get(2) ?? []).map(text),
  }))

interface Call {
  url: string
  headers: Headers
  body: Uint8Array
}

/** Stubs fetch with a shard whose stream stays open. log has each call's method and each status. */
function fakeShard() {
  const calls: Call[] = []
  const log: string[] = []
  let push = (_message: Uint8Array) => {}
  vi.stubGlobal('fetch', async (url: string, init: RequestInit) => {
    calls.push({ url, headers: new Headers(init.headers), body: init.body as Uint8Array })
    log.push(url.slice(service.length + 1))
    if (!url.endsWith('/StartEventStream')) {
      return new Response(new Uint8Array(), { headers: { 'content-type': 'application/proto' } })
    }
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        push = (message) => controller.enqueue(envelope(message))
        push(new Uint8Array()) // Cardinal's empty first response
      },
    })
    return new Response(body, { headers: { 'content-type': 'application/connect+proto' } })
  })
  return { calls, log, push: (message: Uint8Array) => push(message) }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

test('streams from and sends to <gatewayUrl>/<project>/<shardId>, as the dev player', async () => {
  vi.spyOn(console, 'log').mockImplementation(() => {}) // the SDK logs the selected region
  const shard = fakeShard()
  const got: Sample[] = []

  const net = new ShardConnection('dev-player-1', where)
  net.onStatus(({ state }) => shard.log.push(state))
  const subscribed = await net.subscribe([
    on(SampleSync, (s) => got.push(s)),
    on(OtherSync, () => {}),
  ])
  await vi.waitFor(() => expect(net.status.state).toBe('connected'))

  // The stream opens before subscribe(), so the SDK adds the names before reporting connected.
  expect(subscribed).toBeNull()
  expect(shard.log).toEqual(['connecting', 'StartEventStream', 'SubscribeEvents', 'connected'])
  const [stream, subscribe] = shard.calls
  expect(stream?.url).toBe(`${service}/StartEventStream`)
  expect(stream?.headers.get('content-type')).toBe('application/connect+proto')
  expect(stream?.headers.get('x-player-id')).toBe('dev-player-1')
  expect(stream?.body).toEqual(envelope(new Uint8Array()))
  expect(subscribe?.url).toBe(`${service}/SubscribeEvents`)
  expect(subscribe?.headers.get('x-player-id')).toBe('dev-player-1')
  expect(subscriptions(subscribe?.body)).toEqual([
    { address: arena, events: ['sample_sync', 'other_sync'] },
  ])

  // StartEventStreamResponse: event = 2 (name = 1, payload = 2). A uint64 stays a bigint.
  shard.push(field(2, concat(field(1, 'sample_sync'), field(2, encodePayload(sample)))))
  await vi.waitFor(() => expect(got).toHaveLength(1))
  expect(got[0]).toStrictEqual(sample)

  // The payload goes out as encodePayload's bytes, which wire_fixtures.test.ts pins for Input.
  net.send(MoveCommand, sample)
  await vi.waitFor(() => expect(shard.calls).toHaveLength(3))
  const send = shard.calls[2]
  expect(send?.url).toBe(`${service}/SendCommand`)
  expect(send?.headers.get('content-type')).toBe('application/proto')
  expect(send?.headers.get('x-player-id')).toBe('dev-player-1')
  // SendCommandRequest: command = 1. Command: name = 1, address = 2, payload = 4.
  const command = one(send?.body, 1)
  expect(text(one(command, 1))).toBe('move')
  expect(address(one(command, 2))).toEqual(arena)
  expect(one(command, 4)).toEqual(encodePayload(sample))
})
