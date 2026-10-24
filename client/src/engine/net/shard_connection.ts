import { Code, createConfig, createSDK } from '@argus-labs/sdk'
import type { ConnectError, ShardClient, ShardStatus } from '@argus-labs/sdk'

/** A Cardinal event or command: its Go Name(), typed with its payload. */
export interface Message<T> {
  readonly name: string
  /** Never set. It only carries T. */
  readonly payload?: T
}

/** A handler for one event, from on. */
export interface EventHandler {
  readonly name: string
  readonly handle: (payload: unknown) => void
}

/** Calls handle with each event payload: decoded MessagePack, cast here and nowhere else. */
export function on<T>(event: Message<T>, handle: (payload: T) => void): EventHandler {
  return { name: event.name, handle: (payload) => handle(payload as T) }
}

/** Where a shard listens: <gatewayUrl>/<project>/<shardId>. */
export interface ShardAddress {
  gatewayUrl: string
  project: string
  shardId: string
}

/** One shard, as a DEV-auth player. The SDK reconnects by itself. */
export class ShardConnection {
  readonly #shard: ShardClient
  readonly #warned = new Set<string>()

  constructor(playerId: string, { gatewayUrl, project, shardId }: ShardAddress) {
    const local = { gatewayUrl, displayName: 'Local' }
    const sdk = createSDK(createConfig({ regions: { local }, devPlayerId: playerId }))
    this.#shard = sdk.shard.register({ project, shardId })
  }

  get status(): ShardStatus {
    return this.#shard.getStatus()
  }

  /** Calls listener with the status now and on every change. */
  onStatus(listener: (status: ShardStatus) => void): void {
    this.#shard.onStatusChange(listener)
    listener(this.#shard.getStatus())
  }

  /** Subscribes once; the SDK resends the subscription on reconnect. */
  async subscribe(handlers: readonly EventHandler[]): Promise<ConnectError | null> {
    const onError = (error: ConnectError) => this.#warnOnce('event', error)
    const { error } = await this.#shard.subscribeEvents(
      handlers.map(({ name, handle }) => ({ name, callbacks: { onSuccess: handle, onError } })),
    )
    return error
  }

  /** Sends without waiting. A failure is only logged. */
  send<T>(command: Message<T>, payload: T): void {
    const { name } = command
    void this.#shard.sendCommand({ name, payload }).then(({ error }) => {
      if (error) this.#warnOnce(name, error)
    })
  }

  // One warning per kind of error, so a server that's down doesn't flood the console.
  #warnOnce(what: string, error: ConnectError): void {
    const kind = `${what} ${Code[error.code]}`
    if (this.#warned.has(kind)) return
    this.#warned.add(kind)
    console.warn(`${kind}: ${error.message} (not logged again)`)
  }
}
