import { Link } from 'react-router'

/** Stand-in for pages built in later milestones. */
export default function Placeholder({ title, milestone }: { title: string; milestone: string }) {
  return (
    <div className="card space-y-3 text-center">
      <h1 className="text-xl font-bold">{title}</h1>
      <p className="text-stone-600">Coming in {milestone}.</p>
      <Link to="/" className="inline-block font-semibold text-orange-700 underline">
        Home
      </Link>
    </div>
  )
}
