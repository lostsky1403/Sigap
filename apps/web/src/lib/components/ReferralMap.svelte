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

		map = new maplibregl.Map({
			container,
			style: 'https://demotiles.maplibre.org/style.json',
			center: [106.8, -6.2], // approx Indonesia / Jakarta area
			zoom: 4.5,
			attributionControl: false
		});

		map.addControl(new maplibregl.NavigationControl(), 'top-right');

		// Target pin (red) - penuh
		if (target && target.lat != null && target.lon != null) {
			const el = document.createElement('div');
			el.style.width = '18px';
			el.style.height = '18px';
			el.style.background = '#ef4444';
			el.style.border = '2px solid #fff';
			el.style.borderRadius = '9999px';
			el.style.boxShadow = '0 0 0 3px rgba(239,68,68,0.3)';

			const m = new maplibregl.Marker({ element: el })
				.setLngLat([target.lon, target.lat])
				.setPopup(new maplibregl.Popup({ offset: 12 }).setHTML(`<strong>${target.name}</strong><br><span style="color:#ef4444">PENUH — rujukan tersedia</span>`))
				.addTo(map);
			markers.push(m);
		}

		// Alt pins (emerald green) - clickable for auto route
		alternatives.forEach((f) => {
			if (f.lat == null || f.lon == null) return;
			const el = document.createElement('div');
			el.style.width = '16px';
			el.style.height = '16px';
			el.style.background = '#059669'; // emerald-600
			el.style.border = '2px solid #fff';
			el.style.borderRadius = '9999px';
			el.style.cursor = 'pointer';
			el.style.boxShadow = '0 0 0 3px rgba(5,150,105,0.25)';

			const m = new maplibregl.Marker({ element: el })
				.setLngLat([f.lon, f.lat])
				.setPopup(new maplibregl.Popup({ offset: 10 }).setHTML(`<strong>${f.name}</strong><br><span style="color:#059669">Tersedia • klik untuk rujuk otomatis</span>`))
				.addTo(map);

			el.addEventListener('click', () => {
				onSelect(f);
			});
			markers.push(m);
		});

		// Fit bounds if we have points
		if (markers.length > 0) {
			const bounds = new maplibregl.LngLatBounds();
			markers.forEach((m) => bounds.extend(m.getLngLat()));
			map.fitBounds(bounds, { padding: 40, maxZoom: 7 });
		}
	});

	onDestroy(() => {
		markers.forEach((m) => m.remove());
		if (map) map.remove();
	});
</script>

<div bind:this={container} class="w-full rounded-2xl border border-emerald-200 dark:border-emerald-800 overflow-hidden shadow-sm" style="height: 320px; min-height: 280px;"></div>
<div class="mt-1 text-[10px] text-center text-slate-500 dark:text-slate-400">
	Peta interaktif (MapLibre / mapcn style). Pin merah = tujuan penuh. Pin hijau emerald = alternatif (klik untuk ambil antrean otomatis).
</div>
