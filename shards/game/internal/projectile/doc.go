// Package projectile owns shots. It launches one when an ability with a launcher is cast, flies it,
// resolves its first hit, and syncs it. A shot isn't a physics body: each tick it sweeps its path
// against walls (CircleSweep) and against Body and Echo circles, and the first hit wins. Shots pass
// through their own team; an enemy Body blocks a shot, and an enemy Echo takes its damage.
package projectile
