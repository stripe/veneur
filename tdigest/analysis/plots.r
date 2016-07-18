library(ggplot2)
library(data.table)
library(scales)

deviations <- data.table::fread('deviations.csv')
centroids <- data.table::fread('centroid-errors.csv')
errors <- data.table::fread('errors.csv')

png(filename = 'sizes.png', width = 2560, height = 1440)
system.time(print(
  ggplot(data = centroids,
         aes(x     = approx_cdf,
             y     = weight,
             color = dist)) +
  geom_point(alpha = 0.1,
             shape = 3) +
  ggtitle('Centroid Weights') +
  scale_x_continuous(name   = 'Quantile',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n=10)) +
  scale_y_continuous(name   = 'Weight (# of Samples)',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n=10))
))
dev.off()

png(filename = 'mean-uniformity.png', width = 2560, height = 1440)
system.time(print(
  ggplot(data = centroids,
         aes(x     = (left / (left + right) - 0.5),
             color = dist)) +
  geom_freqpoly(binwidth = 0.001,
                center   = 0,
                alpha    = 0.5) +
  ggtitle('Mean Uniformity Between Adjacent Centroids') +
  scale_x_continuous(name   = 'Normalized Distance to Adjacent Centroid',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10)) +
  scale_y_continuous(name   = '# of Centroids',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10))
))
dev.off()

png(filename = 'sample-deviations.png', width = 2560, height = 1440)
system.time(print(
  ggplot(data = deviations,
         aes(x     = deviation,
             color = dist)) +
  geom_freqpoly(binwidth = 0.01,
                center   = 0,
                alpha    = 0.5) +
  geom_vline(xintercept = c(-0.5, 0.5),
             color = 'darkgray') +
  ggtitle('Deviation of Samples Assigned To a Single Centroid (Fig2)') +
  scale_x_continuous(name   = 'Normalized Distance to Adjacent Centroid',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10)) +
  scale_y_continuous(name   = '# of Samples',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10))
))
dev.off()

png(filename = 'error-cdf.png', width = 2560, height = 1440)
system.time(print(
  ggplot(data = errors,
         aes(x     = Q,
             group = interaction(Q, dist),
             y     = (approx_Q - Q) * 1e6,
             color = dist)) +
  geom_boxplot(position      = 'identity',
               fill          = 'white',
               alpha         = 0,
               outlier.shape = 3) +
  ggtitle('Absolute Error of CDF Estimate (Fig3)') +
  scale_x_continuous(name   = 'Quantile',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10)) +
  scale_y_continuous(name   = 'CDF Absolute Error (ppm)',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10))
))
dev.off()

png(filename = 'mean-uniformity-by-quantile.png', width = 2560, height = 1440)
system.time(print(
  ggplot(data = centroids,
         aes(x     = approx_cdf,
             group = interaction(cut_width(approx_cdf, 0.001, boundary = 0), dist),
             y     = (left / (left + right) - 0.5),
             color = dist)) +
  geom_boxplot(position      = 'identity',
               fill          = 'white',
               alpha         = 0,
               outlier.shape = 3) +
  ggtitle('Mean Uniformity Between Adjacent Centroids, by Quantile') +
  scale_x_continuous(name   = 'Quantile',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10)) +
  scale_y_continuous(name   = 'Normalized Distance to Adjacent Centroid',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10))
))
dev.off()

png(filename = 'sample-deviations-by-quantile.png', width = 2560, height = 1440)
system.time(print(
  ggplot(data = deviations,
         aes(x     = Q,
             group = interaction(cut_width(Q, 0.001, boundary = 0), dist),
             y     = deviation,
             color = dist)) +
  geom_boxplot(position      = 'identity',
               fill          = 'white',
               alpha         = 0,
               outlier.shape = 3) +
  geom_hline(yintercept = c(-0.5, 0.5),
             color = 'darkgray') +
  ggtitle('Deviation of Samples Assigned To a Single Centroid (Fig2), by Quantile') +
  scale_x_continuous(name   = 'Quantile',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10)) +
  scale_y_continuous(name   = 'Normalized Distance to Adjacent Centroid',
                     expand = c(0.01, 0),
                     breaks = scales::extended_breaks(n = 10))
))
dev.off()
