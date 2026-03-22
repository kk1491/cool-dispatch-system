import { ServiceZone } from '../types';

export function matchZoneByAddress(address: string, zones: ServiceZone[]): string | undefined {
  for (const zone of zones) {
    for (const district of zone.districts) {
      if (address.includes(district)) {
        return zone.id;
      }
    }
  }
  return undefined;
}
