import { useState } from 'react';
import { Plus, X, MapPin, Users, Trash2, Edit2, Check } from 'lucide-react';
import { cn } from '../lib/utils';
import { ServiceZone, User } from '../types';
import { TAIPEI_DISTRICTS, NEW_TAIPEI_DISTRICTS } from '../data/constants';
import { Button, Card } from './shared';

interface ZoneManagementProps {
  zones: ServiceZone[];
  technicians: User[];
  onUpdateZones: (zones: ServiceZone[]) => void;
}

export default function ZoneManagement({ zones, technicians, onUpdateZones }: ZoneManagementProps) {
  const [editingZone, setEditingZone] = useState<ServiceZone | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [newZoneName, setNewZoneName] = useState('');
  const [selectedDistricts, setSelectedDistricts] = useState<string[]>([]);
  const [selectedTechIds, setSelectedTechIds] = useState<number[]>([]);
  const [districtTab, setDistrictTab] = useState<'taipei' | 'new_taipei'>('taipei');

  const allDistricts = districtTab === 'taipei' ? TAIPEI_DISTRICTS : NEW_TAIPEI_DISTRICTS;

  const resetForm = () => {
    setNewZoneName('');
    setSelectedDistricts([]);
    setSelectedTechIds([]);
    setIsCreating(false);
    setEditingZone(null);
  };

  const handleStartEdit = (zone: ServiceZone) => {
    setEditingZone(zone);
    setNewZoneName(zone.name);
    setSelectedDistricts([...zone.districts]);
    setSelectedTechIds([...zone.assigned_technician_ids]);
    setIsCreating(false);
  };

  const handleStartCreate = () => {
    resetForm();
    setIsCreating(true);
  };

  const toggleDistrict = (district: string) => {
    setSelectedDistricts(prev =>
      prev.includes(district) ? prev.filter(d => d !== district) : [...prev, district]
    );
  };

  const toggleTech = (techId: number) => {
    setSelectedTechIds(prev =>
      prev.includes(techId) ? prev.filter(id => id !== techId) : [...prev, techId]
    );
  };

  const handleSave = () => {
    if (!newZoneName.trim() || selectedDistricts.length === 0) return;

    if (editingZone) {
      const updated: ServiceZone = {
        ...editingZone,
        name: newZoneName.trim(),
        districts: selectedDistricts,
        assigned_technician_ids: selectedTechIds,
      };
      onUpdateZones(zones.map(z => z.id === editingZone.id ? updated : z));
    } else {
      const newZone: ServiceZone = {
        id: `zone-${Date.now()}`,
        name: newZoneName.trim(),
        districts: selectedDistricts,
        assigned_technician_ids: selectedTechIds,
      };
      onUpdateZones([...zones, newZone]);
    }
    resetForm();
  };

  const handleDelete = (zoneId: string) => {
    if (confirm('確定要刪除這個服務區域嗎？')) {
      onUpdateZones(zones.filter(z => z.id !== zoneId));
    }
  };

  const techs = technicians.filter(t => t.role === 'technician');

  const getAssignedZoneCount = (techId: number) => {
    return zones.filter(z => z.assigned_technician_ids.includes(techId)).length;
  };

  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="text-sm text-slate-500">管理服務區域，指定各區負責師傅</p>
        </div>
        <Button data-testid="button-create-zone" onClick={handleStartCreate}>
          <Plus className="w-4 h-4" />
          新增區域
        </Button>
      </div>

      <div className="grid md:grid-cols-2 xl:grid-cols-3 gap-6">
        {zones.map(zone => {
          const assignedTechs = techs.filter(t => zone.assigned_technician_ids.includes(t.id));
          return (
            <Card key={zone.id} className="p-6 space-y-4" data-testid={`card-zone-${zone.id}`}>
              <div className="flex items-start justify-between gap-2">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-blue-50 rounded-md flex items-center justify-center">
                    <MapPin className="w-5 h-5 text-blue-600" />
                  </div>
                  <div>
                    <h3 className="font-bold text-slate-900" data-testid={`text-zone-name-${zone.id}`}>{zone.name}</h3>
                    <p className="text-xs text-slate-400">{zone.districts.length} 個行政區</p>
                  </div>
                </div>
                <div className="flex gap-1">
                  <button
                    onClick={() => handleStartEdit(zone)}
                    data-testid={`button-edit-zone-${zone.id}`}
                    className="p-2 text-slate-400 hover:text-slate-600 transition-colors rounded-lg"
                  >
                    <Edit2 className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => handleDelete(zone.id)}
                    data-testid={`button-delete-zone-${zone.id}`}
                    className="p-2 text-slate-400 hover:text-red-500 transition-colors rounded-lg"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>

              <div className="flex flex-wrap gap-1.5">
                {zone.districts.map(d => (
                  <span key={d} className="px-2 py-0.5 bg-slate-100 text-slate-600 rounded-md text-xs font-medium">
                    {d}
                  </span>
                ))}
              </div>

              <div className="border-t border-slate-100 pt-4">
                <div className="flex items-center gap-2 mb-2">
                  <Users className="w-4 h-4 text-slate-400" />
                  <span className="text-xs font-bold text-slate-400 uppercase tracking-wider">負責師傅</span>
                </div>
                {assignedTechs.length === 0 ? (
                  <p className="text-xs text-slate-400">尚未指派師傅</p>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    {assignedTechs.map(t => (
                      <div key={t.id} className="flex items-center gap-2 bg-slate-50 rounded-lg px-3 py-1.5" data-testid={`badge-tech-${t.id}-zone-${zone.id}`}>
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-white text-[10px] font-bold" style={{ backgroundColor: t.color || '#6366f1' }}>
                          {t.name[0]}
                        </div>
                        <span className="text-xs font-medium text-slate-700">{t.name}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </Card>
          );
        })}
      </div>

      {zones.length === 0 && !isCreating && (
        <div className="text-center py-20 bg-white rounded-lg border border-slate-100">
          <div className="w-16 h-16 bg-slate-100 rounded-full flex items-center justify-center mx-auto mb-4">
            <MapPin className="text-slate-300 w-8 h-8" />
          </div>
          <p className="text-slate-500 mb-4">尚未建立服務區域</p>
          <Button data-testid="button-create-zone-empty" onClick={handleStartCreate}>
            <Plus className="w-4 h-4" />
            建立第一個區域
          </Button>
        </div>
      )}

      {(isCreating || editingZone) && (
        <div className="fixed inset-0 bg-black/20 backdrop-blur-sm z-[60] flex items-center justify-center p-4">
          <Card className="w-full max-w-2xl max-h-[90vh] overflow-y-auto p-8 space-y-6" data-testid="dialog-zone-form">
            <div className="flex items-center justify-between gap-4">
              <h3 className="text-lg font-bold text-slate-900">
                {editingZone ? '編輯服務區域' : '新增服務區域'}
              </h3>
              <button onClick={resetForm} className="p-2 text-slate-400 hover:text-slate-600 transition-colors" data-testid="button-close-zone-form">
                <X className="w-5 h-5" />
              </button>
            </div>

            <div>
              <label className="block text-sm font-medium text-slate-700 mb-1">區域名稱</label>
              <input
                data-testid="input-zone-name"
                type="text"
                value={newZoneName}
                onChange={e => setNewZoneName(e.target.value)}
                placeholder="例如：台北信義區"
                className="w-full px-4 py-3 rounded-md border border-slate-200 focus:outline-none focus:ring-2 focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">選擇行政區</label>
              <div className="flex gap-2 mb-3">
                <button
                  type="button"
                  data-testid="tab-taipei"
                  onClick={() => setDistrictTab('taipei')}
                  className={cn(
                    "px-4 py-2 rounded-full text-sm font-medium transition-all",
                    districtTab === 'taipei' ? "bg-blue-600 text-white" : "bg-slate-100 text-slate-500"
                  )}
                >
                  台北市
                </button>
                <button
                  type="button"
                  data-testid="tab-new-taipei"
                  onClick={() => setDistrictTab('new_taipei')}
                  className={cn(
                    "px-4 py-2 rounded-full text-sm font-medium transition-all",
                    districtTab === 'new_taipei' ? "bg-blue-600 text-white" : "bg-slate-100 text-slate-500"
                  )}
                >
                  新北市
                </button>
              </div>
              <div className="grid grid-cols-3 sm:grid-cols-4 gap-2">
                {allDistricts.map(d => {
                  const isSelected = selectedDistricts.includes(d);
                  const isUsedByOther = zones.some(z => z.id !== editingZone?.id && z.districts.includes(d));
                  return (
                    <button
                      key={d}
                      type="button"
                      data-testid={`checkbox-district-${d}`}
                      onClick={() => toggleDistrict(d)}
                      disabled={isUsedByOther}
                      className={cn(
                        "px-3 py-2 rounded-md text-sm font-medium transition-all border",
                        isSelected
                          ? "bg-blue-50 border-blue-300 text-blue-700"
                          : isUsedByOther
                            ? "bg-slate-50 border-slate-100 text-slate-300 cursor-not-allowed"
                            : "bg-white border-slate-200 text-slate-600 hover:border-slate-300"
                      )}
                    >
                      {isSelected && <Check className="w-3 h-3 inline mr-1" />}
                      {d}
                      {isUsedByOther && <span className="text-[10px] block text-slate-300">(已指派)</span>}
                    </button>
                  );
                })}
              </div>
              {selectedDistricts.length > 0 && (
                <p className="text-xs text-slate-500 mt-2">已選擇 {selectedDistricts.length} 個行政區</p>
              )}
            </div>

            <div>
              <label className="block text-sm font-medium text-slate-700 mb-2">指定負責師傅</label>
              <div className="space-y-2">
                {techs.map(t => {
                  const isSelected = selectedTechIds.includes(t.id);
                  const otherZoneCount = zones.filter(z => z.id !== editingZone?.id && z.assigned_technician_ids.includes(t.id)).length;
                  return (
                    <button
                      key={t.id}
                      type="button"
                      data-testid={`checkbox-tech-${t.id}`}
                      onClick={() => toggleTech(t.id)}
                      className={cn(
                        "w-full flex items-center gap-3 px-4 py-3 rounded-md border transition-all text-left",
                        isSelected
                          ? "bg-blue-50 border-blue-300"
                          : "bg-white border-slate-200 hover:border-slate-300"
                      )}
                    >
                      <div className="w-8 h-8 rounded-full flex items-center justify-center text-white text-sm font-bold" style={{ backgroundColor: t.color || '#6366f1' }}>
                        {t.name[0]}
                      </div>
                      <div className="flex-1">
                        <p className={cn("text-sm font-medium", isSelected ? "text-blue-700" : "text-slate-700")}>{t.name}</p>
                        <p className="text-xs text-slate-400">
                          {t.skills?.join(', ') || '未設定技能'}
                          {otherZoneCount > 0 && ` · 已在 ${otherZoneCount} 個區域`}
                        </p>
                      </div>
                      {isSelected && (
                        <div className="w-6 h-6 bg-blue-600 rounded-full flex items-center justify-center">
                          <Check className="w-3.5 h-3.5 text-white" />
                        </div>
                      )}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="flex justify-end gap-3 pt-4 border-t border-slate-100">
              <Button variant="outline" onClick={resetForm} data-testid="button-cancel-zone">
                取消
              </Button>
              <Button
                onClick={handleSave}
                data-testid="button-save-zone"
                disabled={!newZoneName.trim() || selectedDistricts.length === 0}
              >
                {editingZone ? '儲存變更' : '建立區域'}
              </Button>
            </div>
          </Card>
        </div>
      )}

      {zones.length > 0 && (
        <Card className="p-6 space-y-4" data-testid="card-zone-overview">
          <h3 className="text-xs font-bold text-slate-400 uppercase tracking-wider">師傅分配總覽</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm text-left text-slate-600">
              <thead className="text-xs text-slate-700 uppercase bg-slate-50">
                <tr>
                  <th className="px-4 py-3">師傅</th>
                  <th className="px-4 py-3">負責區域</th>
                  <th className="px-4 py-3">技能</th>
                  <th className="px-4 py-3">區域數</th>
                </tr>
              </thead>
              <tbody>
                {techs.map(t => {
                  const assignedZones = zones.filter(z => z.assigned_technician_ids.includes(t.id));
                  return (
                    <tr key={t.id} className="bg-white border-b" data-testid={`row-tech-overview-${t.id}`}>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-2">
                          <div className="w-6 h-6 rounded-full flex items-center justify-center text-white text-[10px] font-bold" style={{ backgroundColor: t.color || '#6366f1' }}>
                            {t.name[0]}
                          </div>
                          <span className="font-medium text-slate-900">{t.name}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-wrap gap-1">
                          {assignedZones.length === 0 ? (
                            <span className="text-slate-400 text-xs">未指派</span>
                          ) : (
                            assignedZones.map(z => (
                              <span key={z.id} className="px-2 py-0.5 bg-blue-50 text-blue-600 rounded-md text-xs font-medium">
                                {z.name}
                              </span>
                            ))
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-wrap gap-1">
                          {t.skills?.map(s => (
                            <span key={s} className="px-2 py-0.5 bg-slate-100 text-slate-600 rounded-md text-xs">{s}</span>
                          )) || <span className="text-slate-400 text-xs">-</span>}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-center font-medium">{assignedZones.length}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}

