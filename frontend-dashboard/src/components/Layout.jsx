import { NavLink } from 'react-router-dom'

const nav = [
  { to: '/', label: 'Dashboard' },
  { to: '/activity', label: 'Live Activity' },
  { to: '/analytics', label: 'Analytics' },
  { to: '/sql', label: 'Ask PEP' },
  { to: '/voice-sql', label: 'Voice Assistant' },
  { to: '/replay', label: 'Replay' },
]

export default function Layout({ children }) {
  return (
    <div className="min-h-screen flex flex-col">
      <header className="sticky top-0 z-50 bg-netflix-bg/95 backdrop-blur border-b border-white/10">
        <div className="max-w-7xl mx-auto px-4 py-3 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-netflix-accent font-bold text-xl">PEP</span>
            <span className="text-sm text-netflix-muted hidden sm:inline">
              Personalized Engagement Platform
            </span>
          </div>
          <nav className="flex gap-1 sm:gap-2">
            {nav.map(({ to, label }) => (
              <NavLink
                key={to}
                to={to}
                className={({ isActive }) =>
                  `px-3 py-1.5 rounded text-sm transition-colors ${
                    isActive ? 'bg-netflix-accent text-white' : 'text-netflix-muted hover:text-white'
                  }`
                }
              >
                {label}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>
      <main className="flex-1 max-w-7xl w-full mx-auto px-4 py-6">{children}</main>
    </div>
  )
}
