export const HELM_RELEASE_SECRET_TYPE = 'helm.sh/release.v1';

export const filterSecretsForView = <T extends { type?: string }>(secrets: T[], hideHelm: boolean): T[] => {
    if (!hideHelm) return secrets;
    return secrets.filter(secret => secret.type !== HELM_RELEASE_SECRET_TYPE);
};
