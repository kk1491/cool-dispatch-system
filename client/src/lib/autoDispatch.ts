import { User, Appointment, ACType, ServiceZone } from '../types';
import { parseISO, format } from 'date-fns';

export interface DispatchScore {
  technician: User;
  totalScore: number;
  reasons: {
    zoneMatch: boolean;
    timeAvailable: boolean;
    skillMatch: boolean;
    loadBalance: number;
  };
}

export function getAutoDispatchSuggestions(
  appointment: Appointment,
  technicians: User[],
  appointments: Appointment[],
  zones: ServiceZone[]
): DispatchScore[] {
  const apptDate = parseISO(appointment.scheduled_at);
  const day = apptDate.getDay();
  const time = format(apptDate, 'HH:00');
  const apptDateStr = appointment.scheduled_at.split('T')[0];
  const requiredTypes = appointment.items.map(i => i.type);

  return technicians.map(tech => {
    let score = 0;
    const reasons = {
      zoneMatch: false,
      timeAvailable: false,
      skillMatch: false,
      loadBalance: 0
    };

    const matchedZone = zones.find(z => 
      z.id === appointment.zone_id && z.assigned_technician_ids.includes(tech.id)
    );
    if (matchedZone) {
      reasons.zoneMatch = true;
      score += 30;
    } else if (!appointment.zone_id) {
      score += 10;
    }

    const dayAvail = tech.availability?.find(a => a.day === day);
    if (dayAvail?.slots.includes(time)) {
      reasons.timeAvailable = true;
      score += 30;
    }

    const existingAppts = appointments.filter(a => 
      a.technician_id === tech.id && 
      a.scheduled_at.startsWith(apptDateStr) &&
      a.status !== 'cancelled'
    );

    const hasTimeConflict = existingAppts.some(a => {
      const existingTime = format(parseISO(a.scheduled_at), 'HH:00');
      return existingTime === time;
    });

    if (hasTimeConflict) {
      reasons.timeAvailable = false;
      score -= 50;
    }

    if (tech.skills && tech.skills.length > 0) {
      const matchCount = requiredTypes.filter(t => tech.skills!.includes(t)).length;
      if (matchCount === requiredTypes.length) {
        reasons.skillMatch = true;
        score += 25;
      } else if (matchCount > 0) {
        score += 10;
      }
    } else {
      score += 5;
    }

    const dailyLoad = existingAppts.length;
    reasons.loadBalance = dailyLoad;
    score -= dailyLoad * 5;
    if (dailyLoad === 0) score += 15;

    return {
      technician: tech,
      totalScore: score,
      reasons
    };
  }).sort((a, b) => b.totalScore - a.totalScore);
}
