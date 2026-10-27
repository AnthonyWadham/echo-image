/** The DEV-auth players the home screen offers, one per Body. */
export const Players = ['dev-player-1', 'dev-player-2']

/** The URL that plays as this player. */
export const playUrl = (playerId: string) => `?player=${encodeURIComponent(playerId)}`

/** The home screen: one button per player, each opening the game as that player. */
export function showHome(root: HTMLElement): void {
  const home = document.createElement('main')
  home.id = 'home'
  const title = document.createElement('h1')
  title.textContent = 'Echo'
  home.append(title)
  Players.forEach((id, i) => {
    const button = document.createElement('a')
    button.className = 'play'
    button.href = playUrl(id)
    button.textContent = `Player ${i + 1}`
    home.append(button)
  })
  root.append(home)
}
