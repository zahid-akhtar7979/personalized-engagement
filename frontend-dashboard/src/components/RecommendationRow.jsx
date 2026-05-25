import RecommendationCard from './RecommendationCard'

export default function RecommendationRow({ title, items = [] }) {
  if (!items?.length) {
    return (
      <section className="mb-8 animate-slide-up">
        <h2 className="text-lg font-semibold mb-3">{title}</h2>
        <p className="text-netflix-muted text-sm">Waiting for live recommendations...</p>
      </section>
    )
  }

  return (
    <section className="mb-8 animate-slide-up">
      <h2 className="text-lg font-semibold mb-3 flex items-center gap-2">
        {title}
        <span className="text-xs font-normal text-netflix-muted">({items.length})</span>
      </h2>
      <div className="flex gap-4 overflow-x-auto pb-2 scrollbar-thin">
        {items.map((item) => (
          <RecommendationCard key={`${title}-${item.itemId}`} item={item} />
        ))}
      </div>
    </section>
  )
}
