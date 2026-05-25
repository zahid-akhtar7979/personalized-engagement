export default function RecommendationCard({ item, onClick }) {
  return (
    <div
      onClick={onClick}
      className="flex-shrink-0 w-44 sm:w-52 group cursor-pointer animate-fade-in"
    >
      <div className="relative overflow-hidden rounded-lg bg-netflix-card aspect-[3/2] transition-transform duration-300 group-hover:scale-105 group-hover:shadow-xl group-hover:shadow-netflix-accent/20">
        <img
          src={item.imageUrl || `https://picsum.photos/seed/${item.itemId}/300/200`}
          alt={item.title}
          className="w-full h-full object-cover"
          loading="lazy"
        />
        <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
        <span className="absolute top-2 right-2 bg-netflix-accent/90 text-xs px-2 py-0.5 rounded font-medium">
          {item.score?.toFixed(1) || '—'}
        </span>
      </div>
      <h4 className="mt-2 text-sm font-medium truncate">{item.title}</h4>
      <p className="text-xs text-netflix-muted truncate">{item.category}</p>
      <p className="text-xs text-netflix-accent/80 truncate mt-0.5">{item.reason}</p>
    </div>
  )
}
