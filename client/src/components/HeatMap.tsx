import { useState, useEffect, useRef, useMemo } from 'react';
import { Calendar, Filter, MapPin, X } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import 'leaflet.heat';
import { cn } from '../lib/utils';
import { Appointment } from '../types';
import { DISTRICT_COORDS } from '../data/constants';
import { Button, Card } from './shared';

function getDistrictFromAddress(address: string): string | null {
  for (const district of Object.keys(DISTRICT_COORDS)) {
    if (address.includes(district)) {
      return district;
    }
  }
  return null;
}

interface HeatMapProps {
  appointments: Appointment[];
}

type HeatLayerFactory = typeof L & {
  heatLayer?: (
    latlngs: Array<[number, number, number]>,
    options: {
      radius: number;
      blur: number;
      maxZoom: number;
      max: number;
      gradient: Record<number, string>;
    }
  ) => L.Layer;
};

export default function HeatMap({ appointments }: HeatMapProps) {
  const mapRef = useRef<HTMLDivElement>(null);
  const mapInstanceRef = useRef<L.Map | null>(null);
  const heatLayerRef = useRef<L.Layer | null>(null);
  const markersRef = useRef<L.LayerGroup | null>(null);

  const [dateRange, setDateRange] = useState<{ start: string; end: string }>({ start: '', end: '' });
  const [statusFilter, setStatusFilter] = useState<Appointment['status'] | 'all'>('all');
  const [selectedDistrict, setSelectedDistrict] = useState<string | null>(null);

  const filteredAppointments = useMemo(() => {
    return appointments.filter(appt => {
      const matchesStatus = statusFilter === 'all' || appt.status === statusFilter;
      const apptDate = appt.scheduled_at.split('T')[0];
      const matchesDate =
        (!dateRange.start || apptDate >= dateRange.start) &&
        (!dateRange.end || apptDate <= dateRange.end);
      return matchesStatus && matchesDate;
    });
  }, [appointments, statusFilter, dateRange]);

  const districtStats = useMemo(() => {
    const stats: Record<string, { count: number; appointments: Appointment[] }> = {};
    for (const appt of filteredAppointments) {
      const district = getDistrictFromAddress(appt.address);
      if (district && DISTRICT_COORDS[district]) {
        if (!stats[district]) {
          stats[district] = { count: 0, appointments: [] };
        }
        stats[district].count++;
        stats[district].appointments.push(appt);
      }
    }
    return stats;
  }, [filteredAppointments]);

  const heatData = useMemo(() => {
    return Object.entries(districtStats).map(([district, stat]) => {
      const coords = DISTRICT_COORDS[district];
      return [coords.lat, coords.lng, stat.count] as [number, number, number];
    });
  }, [districtStats]);

  useEffect(() => {
    if (!mapRef.current || mapInstanceRef.current) return;

    const map = L.map(mapRef.current, {
      center: [25.033, 121.5654],
      zoom: 12,
      zoomControl: true,
    });

    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap contributors',
      maxZoom: 18,
    }).addTo(map);

    mapInstanceRef.current = map;
    markersRef.current = L.layerGroup().addTo(map);

    return () => {
      map.remove();
      mapInstanceRef.current = null;
    };
  }, []);

  useEffect(() => {
    const map = mapInstanceRef.current;
    if (!map) return;

    if (heatLayerRef.current) {
      map.removeLayer(heatLayerRef.current);
      heatLayerRef.current = null;
    }

    if (heatData.length > 0) {
      try {
        // leaflet.heat 未提供稳定 TS 类型，这里用局部窄接口承接插件扩展，避免退回 any。
        const heat = (L as HeatLayerFactory).heatLayer?.(heatData, {
          radius: 35,
          blur: 25,
          maxZoom: 15,
          max: Math.max(...heatData.map(d => d[2]), 1),
          gradient: {
            0.2: '#2563eb',
            0.4: '#3b82f6',
            0.6: '#f59e0b',
            0.8: '#ef4444',
            1.0: '#dc2626',
          },
        });
        if (heat) {
          heat.addTo(map);
          heatLayerRef.current = heat;
        } else {
          console.warn('leaflet.heat not available, using markers fallback');
        }
      } catch (e) {
        console.warn('leaflet.heat not available, using markers fallback');
      }
    }

    if (markersRef.current) {
      markersRef.current.clearLayers();
    }

    Object.entries(districtStats).forEach(([district, stat]) => {
      const coords = DISTRICT_COORDS[district];
      if (!coords || !markersRef.current) return;

      const size = Math.min(40, 20 + stat.count * 4);
      const icon = L.divIcon({
        className: 'custom-marker',
        html: `<div style="
          width: ${size}px; height: ${size}px;
          background: rgba(79, 70, 229, 0.85);
          border: 2px solid white;
          border-radius: 50%;
          display: flex; align-items: center; justify-content: center;
          color: white; font-size: 12px; font-weight: bold;
          box-shadow: 0 2px 8px rgba(0,0,0,0.3);
          cursor: pointer;
        ">${stat.count}</div>`,
        iconSize: [size, size],
        iconAnchor: [size / 2, size / 2],
      });

      const marker = L.marker([coords.lat, coords.lng], { icon });
      marker.on('click', () => {
        setSelectedDistrict(district);
      });
      marker.bindTooltip(district, { direction: 'top', offset: [0, -size / 2] });
      markersRef.current.addLayer(marker);
    });
  }, [heatData, districtStats]);

  const selectedDistrictData = selectedDistrict ? districtStats[selectedDistrict] : null;

  return (
    <div className="space-y-6" data-testid="view-heatmap">
      <div className="flex flex-col md:flex-row gap-4 items-start md:items-center">
        <div className="flex gap-2 overflow-x-auto pb-2 scrollbar-hide flex-wrap">
          {(['all', 'pending', 'assigned', 'arrived', 'completed'] as const).map((s) => (
            <button
              key={s}
              onClick={() => setStatusFilter(s)}
              data-testid={`heatmap-filter-status-${s}`}
              className={cn(
                "px-4 py-2 rounded-full text-sm font-medium transition-all whitespace-nowrap",
                statusFilter === s
                  ? "bg-blue-600 text-white shadow-sm"
                  : "bg-white text-slate-500 border border-slate-100 hover:border-slate-200"
              )}
            >
              {s === 'all' ? '全部' : s === 'pending' ? '待指派' : s === 'assigned' ? '已分派' : s === 'arrived' ? '清洗中' : '已完成'}
            </button>
          ))}
        </div>

        <div className="flex gap-1 items-center bg-white border border-slate-100 rounded-md px-2">
          <Calendar className="w-4 h-4 text-slate-400" />
          <input
            type="date"
            data-testid="heatmap-date-start"
            className="px-2 py-2 text-sm focus:outline-none bg-transparent"
            value={dateRange.start}
            onChange={e => setDateRange({ ...dateRange, start: e.target.value })}
          />
          <span className="text-slate-300">~</span>
          <input
            type="date"
            data-testid="heatmap-date-end"
            className="px-2 py-2 text-sm focus:outline-none bg-transparent"
            value={dateRange.end}
            onChange={e => setDateRange({ ...dateRange, end: e.target.value })}
          />
        </div>

        <Button
          variant="outline"
          className="px-3 py-2 rounded-md text-xs"
          data-testid="heatmap-reset-filters"
          onClick={() => {
            setStatusFilter('all');
            setDateRange({ start: '', end: '' });
            setSelectedDistrict(null);
          }}
        >
          重設
        </Button>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <Card className="p-4 bg-blue-50 border-blue-100/50">
          <p className="text-[10px] font-bold text-blue-400 uppercase tracking-wider mb-1">涵蓋區域</p>
          <p className="text-2xl font-bold text-blue-900" data-testid="text-heatmap-districts">{Object.keys(districtStats).length}</p>
        </Card>
        <Card className="p-4 bg-amber-50 border-amber-100/50">
          <p className="text-[10px] font-bold text-amber-400 uppercase tracking-wider mb-1">訂單總數</p>
          <p className="text-2xl font-bold text-amber-900" data-testid="text-heatmap-total">{filteredAppointments.length}</p>
        </Card>
        <Card className="p-4 bg-emerald-50 border-emerald-100/50">
          <p className="text-[10px] font-bold text-emerald-400 uppercase tracking-wider mb-1">最熱區域</p>
          <p className="text-lg font-bold text-emerald-900 truncate" data-testid="text-heatmap-hottest">
            {Object.entries(districtStats).sort((a, b) => b[1].count - a[1].count)[0]?.[0] || '-'}
          </p>
        </Card>
        <Card className="p-4 bg-rose-50 border-rose-100/50">
          <p className="text-[10px] font-bold text-rose-400 uppercase tracking-wider mb-1">最多訂單</p>
          <p className="text-2xl font-bold text-rose-900" data-testid="text-heatmap-max">
            {Object.entries(districtStats).sort((a, b) => b[1].count - a[1].count)[0]?.[1]?.count || 0}
          </p>
        </Card>
      </div>

      <div className="relative">
        <Card className="overflow-hidden">
          <div ref={mapRef} className="w-full h-[500px] md:h-[600px]" data-testid="heatmap-container" />
        </Card>

        {selectedDistrict && selectedDistrictData && (
          <div className="absolute top-4 right-4 z-[1000] w-80">
            <Card className="p-4 bg-white/95 backdrop-blur-sm shadow-lg">
              <div className="flex justify-between items-center mb-3">
                <div className="flex items-center gap-2">
                  <MapPin className="w-4 h-4 text-blue-600" />
                  <h4 className="font-bold text-slate-900" data-testid="text-selected-district">{selectedDistrict}</h4>
                </div>
                <button
                  onClick={() => setSelectedDistrict(null)}
                  data-testid="button-close-district-info"
                  className="text-slate-400 hover:text-slate-600 transition-colors"
                >
                  <X className="w-4 h-4" />
                </button>
              </div>
              <p className="text-sm text-slate-500 mb-3" data-testid="text-district-count">
                共 {selectedDistrictData.count} 筆訂單
              </p>
              <div className="space-y-2 max-h-48 overflow-y-auto">
                {selectedDistrictData.appointments.map(appt => (
                  <div
                    key={appt.id}
                    className="flex justify-between items-center text-sm bg-slate-50 p-2 rounded-lg"
                    data-testid={`heatmap-appt-${appt.id}`}
                  >
                    <div>
                      <p className="font-medium text-slate-900">{appt.customer_name}</p>
                      <p className="text-xs text-slate-500">
                        {format(parseISO(appt.scheduled_at), 'MM/dd HH:mm')}
                      </p>
                    </div>
                    <span className={cn(
                      "px-2 py-0.5 rounded-full text-xs font-bold",
                      appt.status === 'completed' ? 'bg-emerald-50 text-emerald-700' :
                      appt.status === 'pending' ? 'bg-amber-50 text-amber-700' :
                      appt.status === 'assigned' ? 'bg-blue-50 text-blue-700' :
                      appt.status === 'arrived' ? 'bg-violet-50 text-violet-700' :
                      'bg-rose-50 text-rose-700'
                    )}>
                      {appt.status === 'completed' ? '已完成' :
                       appt.status === 'pending' ? '待指派' :
                       appt.status === 'assigned' ? '已分派' :
                       appt.status === 'arrived' ? '清洗中' : '取消'}
                    </span>
                  </div>
                ))}
              </div>
            </Card>
          </div>
        )}
      </div>

      <Card className="p-6">
        <h4 className="text-xs font-bold text-slate-400 uppercase tracking-wider mb-4">各區域訂單統計</h4>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
          {Object.entries(districtStats)
            .sort((a, b) => b[1].count - a[1].count)
            .map(([district, stat]) => (
              <button
                key={district}
                onClick={() => {
                  setSelectedDistrict(district);
                  const coords = DISTRICT_COORDS[district];
                  if (coords && mapInstanceRef.current) {
                    mapInstanceRef.current.setView([coords.lat, coords.lng], 14, { animate: true });
                  }
                }}
                data-testid={`heatmap-district-${district}`}
                className={cn(
                  "p-3 rounded-md text-left transition-all border",
                  selectedDistrict === district
                    ? "bg-blue-50 border-blue-200"
                    : "bg-slate-50 border-slate-100 hover:border-slate-200"
                )}
              >
                <p className="text-sm font-bold text-slate-900">{district}</p>
                <p className="text-xs text-slate-500">{stat.count} 筆訂單</p>
              </button>
            ))}
        </div>
        {Object.keys(districtStats).length === 0 && (
          <p className="text-center text-slate-400 py-8">目前沒有符合條件的訂單資料</p>
        )}
      </Card>
    </div>
  );
}
