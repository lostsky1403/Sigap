<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import maplibregl from 'maplibre-gl';
	import 'maplibre-gl/dist/maplibre-gl.css';

	// ReferralMap (mapcn-inspired interactive MapLibre + emerald/red pins)
	// Target (full) = red pin; Alternatives (available) = emerald pin
	// Click green pin => onSelect(fac) for one-click auto-routing / queue

	export let target: any = null;           // {id, name, lat, lon, ...}
	export let alternatives: any[] = [];     // list of alt fac with lat/lon
	export let onSelect: (fac: any) => void = () => {};

	let container: HTMLDivElement;
	let map: maplibregl.Map | null = null;
	let markers: maplibregl.Marker[] = [];

	onMount(() => {
		if (!container) return;

		// Init ONLY in onMount for CSR (no SSR map render)
		// CSS imported in <script> as required (maplibre-gl/dist/maplibre-gl.css)
		// Use dark CartoDB tiles per instruction for dark theme match
		map = new maplibregl.Map({
			container,
			style: {
				version: 8,
				sources: {
					'carto-dark': {
						type: 'raster',
						tiles: ['https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png'],
						tileSize: 256
					}
				},
				layers: [{
					id: 'carto-dark-layer',
					type: 'raster',
					source: 'carto-dark',
					minzoom: 0,
					maxzoom: 22
				}]
			},
			center: [106.8, -6.2], // approx Indonesia / Jakarta area
			zoom: 4.5,
			attributionControl: false
		});

		map.addControl(new maplibregl.NavigationControl(), 'top-right');

		// Non-null local for TS (map declared |null at top; assignment inside onMount + closures don't narrow automatically)
		const m = map!;

		// Target pin (red teardrop per Stitch design: sharp 0px point at bottom for precise location)
		if (target && target.lat != null && target.lon != null) {
			const el = document.createElement('div');
			el.style.width = '16px';
			el.style.height = '20px';
			el.style.background = '#ef4444';
			el.style.border = '2px solid #fff';
			el.style.borderRadius = '50% 50% 50% 0';
			el.style.transform = 'rotate(-45deg)';
			el.style.boxShadow = '0 0 0 3px rgba(239,68,68,0.3)';
			el.style.cursor = 'default';

			const marker = new maplibregl.Marker({ element: el, anchor: 'bottom' })
				.setLngLat([target.lon, target.lat])
				.setPopup(new maplibregl.Popup({ offset: 12 }).setHTML(`<strong>${target.name}</strong><br><span style="color:#ef4444">PENUH — rujukan tersedia</span>`))
				.addTo(m);
			markers.push(marker);
		}

		// Alt pins (emerald green teardrop) - clickable for auto route
		alternatives.forEach((f) => {
			if (f.lat == null || f.lon == null) return;
			const el = document.createElement('div');
			el.style.width = '14px';
			el.style.height = '18px';
			el.style.background = '#059669'; // emerald-600
			el.style.border = '2px solid #fff';
			el.style.borderRadius = '50% 50% 50% 0';
			el.style.transform = 'rotate(-45deg)';
			el.style.cursor = 'pointer';
			el.style.boxShadow = '0 0 0 3px rgba(5,150,105,0.25)';

			const marker = new maplibregl.Marker({ element: el, anchor: 'bottom' })
				.setLngLat([f.lon, f.lat])
				.setPopup(new maplibregl.Popup({ offset: 10 }).setHTML(`<strong>${f.name}</strong><br><span style="color:#059669">Tersedia • klik untuk rujuk otomatis</span>`))
				.addTo(m);

			el.addEventListener('click', () => {
				onSelect(f);
			});
			markers.push(marker);
		});

		// Robust render: resize + fit ONLY after load to guarantee peta jalanan (dark tiles) visible, not blank
		m.on('load', () => {
			if (!map) return;
			map.resize();
			if (markers.length > 0) {
				const bounds = new maplibregl.LngLatBounds();
				markers.forEach((mk) => bounds.extend(mk.getLngLat()));
				map.fitBounds(bounds, { padding: 40, maxZoom: 7 });
			}
		});
	});

	onDestroy(() => {
		markers.forEach((m) => m.remove());
		if (map) map.remove();
	});
</script>

<div bind:this={container} class="w-full h-[300px] rounded-lg relative border border-slate-700 dark:border-slate-700 overflow-hidden shadow-sm" style="height: 300px;"></div>
<div class="mt-1 text-[10px] text-center text-slate-500 dark:text-slate-400">
	Peta interaktif (MapLibre / mapcn style). Pin merah = tujuan penuh. Pin hijau emerald = alternatif (klik untuk ambil antrean otomatis).
</div>
