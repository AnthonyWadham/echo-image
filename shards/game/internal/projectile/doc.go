// Package projectile owns shots. It launches one for each LaunchIntent, flies it,
// resolves what it meets, and syncs it. A shot isn't a physics body: each tick one Box2D shape cast
// sweeps its path against the map, Bodies and Echoes. What it does on meeting each thing is
// a rule in content, the shot type's own first, then the defaults: by default shots pass through their
// own team, an enemy Body blocks a shot, and an enemy Echo takes its damage (GDD §7).
package projectile
