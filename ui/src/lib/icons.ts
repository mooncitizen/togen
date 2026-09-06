import awsApiGateway from '../../icons/aws/api-gateway.svg?raw';
import awsEcs from '../../icons/aws/ecs.svg?raw';
import awsElasticache from '../../icons/aws/elasticache.svg?raw';
import awsLambda from '../../icons/aws/lambda.svg?raw';
import awsRds from '../../icons/aws/rds.svg?raw';
import awsS3 from '../../icons/aws/s3.svg?raw';
import awsSqs from '../../icons/aws/sqs.svg?raw';
import azureApiManagement from '../../icons/azure/api-management.svg?raw';
import azureCacheRedis from '../../icons/azure/cache-redis.svg?raw';
import azureContainerApps from '../../icons/azure/container-apps.svg?raw';
import azureDatabasePostgresql from '../../icons/azure/database-postgresql.svg?raw';
import azureFunctions from '../../icons/azure/functions.svg?raw';
import azureServiceBus from '../../icons/azure/service-bus.svg?raw';
import azureStorageBlob from '../../icons/azure/storage-blob.svg?raw';
import gcpApiGateway from '../../icons/gcp/api-gateway.svg?raw';
import gcpCloudFunctions from '../../icons/gcp/cloud-functions.svg?raw';
import gcpCloudRun from '../../icons/gcp/cloud-run.svg?raw';
import gcpCloudSql from '../../icons/gcp/cloud-sql.svg?raw';
import gcpCloudStorage from '../../icons/gcp/cloud-storage.svg?raw';
import gcpMemorystore from '../../icons/gcp/memorystore.svg?raw';
import gcpPubsub from '../../icons/gcp/pubsub.svg?raw';
import awsNotice from '../../icons/aws/NOTICE.md?raw';
import azureNotice from '../../icons/azure/NOTICE.md?raw';
import gcpNotice from '../../icons/gcp/NOTICE.md?raw';

// The ids are the ones schema/styles.json names (ADR 0007).
export const bundled: Record<string, string> = {
  'aws/api-gateway': awsApiGateway,
  'aws/ecs': awsEcs,
  'aws/elasticache': awsElasticache,
  'aws/lambda': awsLambda,
  'aws/rds': awsRds,
  'aws/s3': awsS3,
  'aws/sqs': awsSqs,
  'azure/api-management': azureApiManagement,
  'azure/cache-redis': azureCacheRedis,
  'azure/container-apps': azureContainerApps,
  'azure/database-postgresql': azureDatabasePostgresql,
  'azure/functions': azureFunctions,
  'azure/service-bus': azureServiceBus,
  'azure/storage-blob': azureStorageBlob,
  'gcp/api-gateway': gcpApiGateway,
  'gcp/cloud-functions': gcpCloudFunctions,
  'gcp/cloud-run': gcpCloudRun,
  'gcp/cloud-sql': gcpCloudSql,
  'gcp/cloud-storage': gcpCloudStorage,
  'gcp/memorystore': gcpMemorystore,
  'gcp/pubsub': gcpPubsub,
};

export type Paragraph = { quote: boolean; text: string };

export type IconSet = { name: string; notice: Paragraph[] };

export const iconSets: IconSet[] = [
  { name: 'AWS Architecture Icons', notice: paragraphs(awsNotice) },
  { name: 'Google Cloud architecture icons', notice: paragraphs(gcpNotice) },
  { name: 'Azure architecture icons', notice: paragraphs(azureNotice) },
];

// The notices are Markdown for the repository; the panel shows their
// paragraphs and quotes and has a heading of its own.
function paragraphs(notice: string): Paragraph[] {
  return notice
    .trim()
    .split(/\n\s*\n/)
    .filter((block) => !block.startsWith('#'))
    .map((block) => {
      const quote = block.startsWith('>');
      const lines = block.split('\n').map((line) => (quote ? line.replace(/^>\s?/, '') : line));
      return { quote, text: lines.join(' ').trim() };
    });
}
